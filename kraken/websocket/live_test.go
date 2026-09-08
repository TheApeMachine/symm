package websocket

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	gorillawebsocket "github.com/gorilla/websocket"
	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/callback"
	sdk "github.com/krakenfx/api-go/v2/pkg/kraken"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/signal/depthflow"
	"github.com/theapemachine/symm/signal/morphology"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/types"
)

/* testIngress exposes the runtime status a transport release must honor. */
type testIngress struct {
	status *runtime.Status
	frames chan *types.Envelope
}

func (ingress *testIngress) Push(envelope *types.Envelope) {
	if ingress.frames != nil {
		ingress.frames <- envelope
	}
}

func (ingress *testIngress) Status() *runtime.Status { return ingress.status }

func readyTestIngress(channels ...string) map[string]runtime.Ingress[*types.Envelope] {
	ingress := make(map[string]runtime.Ingress[*types.Envelope], len(channels))

	for _, channel := range channels {
		ingress[channel] = &testIngress{
			status: runtime.NewStatus().Transition(runtime.READY),
		}
	}

	return ingress
}

func TestLiveFail(t *testing.T) {
	Convey("Given a required spot session", t, func() {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		live := &Live{
			ctx:    ctx,
			cancel: cancel,
			status: runtime.NewStatus(),
		}
		first := errors.New("spot transport failed")
		second := errors.New("later failure")

		Convey("The first failure should persist and cancel the session", func() {
			live.fail(first)
			live.fail(second)

			So(errors.Is(live.Error(), first), ShouldBeTrue)
			So(errors.Is(live.Error(), second), ShouldBeFalse)
			So(live.Status(), ShouldEqual, runtime.ERROR)
			So(live.ctx.Err(), ShouldEqual, context.Canceled)
		})
	})
}

func TestLiveReconnect(t *testing.T) {
	Convey("Given a ready authenticated Level3 session", t, func() {
		connections := make(chan *gorillawebsocket.Conn, 2)
		frames := make(chan struct {
			connection int64
			payload    []byte
		}, 16)
		serverErrors := make(chan error, 2)
		var accepted atomic.Int64
		var tokenRequests atomic.Int64
		upgrader := gorillawebsocket.Upgrader{
			CheckOrigin: func(*http.Request) bool { return true },
		}
		server := httptest.NewServer(http.HandlerFunc(func(
			writer http.ResponseWriter,
			request *http.Request,
		) {
			connection, err := upgrader.Upgrade(writer, request, nil)

			if err != nil {
				serverErrors <- err

				return
			}

			connectionID := accepted.Add(1)
			connections <- connection

			for {
				_, payload, err := connection.ReadMessage()

				if err != nil {
					return
				}

				if strings.Contains(string(payload), `"channel":"level3"`) {
					if err := connection.WriteMessage(gorillawebsocket.TextMessage, []byte(`{"channel":"level3","type":"snapshot","data":[{"symbol":"BTC/USD","bids":[],"asks":[]}]}`)); err != nil {
						serverErrors <- err
						return
					}
				}

				frames <- struct {
					connection int64
					payload    []byte
				}{connection: connectionID, payload: payload}
			}
		}))
		defer server.Close()

		endpoint := "ws" + strings.TrimPrefix(server.URL, "http")
		previousEndpoint := system.Cfg.WebSocket.Endpoints.Level3
		system.Cfg.WebSocket.Endpoints.Level3 = endpoint
		defer func() { system.Cfg.WebSocket.Endpoints.Level3 = previousEndpoint }()
		client := spot.NewWebSocket()
		client.URL = endpoint
		client.ReconnectWait = 10 * time.Millisecond
		client.REST.Executor = func(request *http.Request) (*http.Response, error) {
			body := `{"error":[],"result":{}}`

			if request.URL.Path == "/0/private/GetWebSocketsToken" {
				attempt := tokenRequests.Add(1)

				if attempt == 2 {
					body = `{"error":["EGeneral:Temporary lockout"],"result":{}}`
				} else {
					body = fmt.Sprintf(
						`{"error":[],"result":{"token":"token-%d","expires":900}}`,
						attempt,
					)
				}
			}

			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(body)),
				Request:    request,
			}, nil
		}
		previousDataPath := viper.GetString("system.data_path")
		viper.Set("system.data_path", t.TempDir())
		t.Cleanup(func() { viper.Set("system.data_path", previousDataPath) })
		t.Setenv("KRAKEN_API_KEY", "test-key")
		t.Setenv("KRAKEN_API_SECRET", "c2VjcmV0")
		live := NewWithClient(
			t.Context(),
			readyTestIngress("level3"),
			nil,
			true,
			endpoint,
			client,
		)
		defer live.Close()

		live.symbols = []string{"BTC/USD"}
		live.MarkReady()
		So(live.subscribeLevel3Group(live), ShouldBeNil)

		awaitChannels := func(connectionID int64, expected ...string) bool {
			remaining := make(map[string]bool, len(expected))

			for _, channel := range expected {
				remaining[channel] = true
			}

			timeout := time.NewTimer(5 * time.Second)
			defer timeout.Stop()

			for len(remaining) > 0 {
				select {
				case frame := <-frames:
					if frame.connection != connectionID {
						continue
					}

					for channel := range remaining {
						if strings.Contains(string(frame.payload), `"channel":"`+channel+`"`) {
							delete(remaining, channel)
						}
					}
				case <-timeout.C:
					return false
				case <-serverErrors:
					return false
				}
			}

			return true
		}

		firstConnection := <-connections
		So(awaitChannels(1, "level3"), ShouldBeTrue)

		for _, failure := range []string{"reader disconnected", "ping failed while still readable"} {
			Convey(failure+" restores a fresh authenticated and seeded session", func() {
				switch failure {
				case "reader disconnected":
					So(firstConnection.Close(), ShouldBeNil)
				case "ping failed while still readable":
					go live.reconnect(errors.New("ping failed"))
				}

				select {
				case <-connections:
				case <-time.After(5 * time.Second):
					So("first reconnect attempt", ShouldEqual, "timed out")
				}

				select {
				case <-connections:
				case <-time.After(5 * time.Second):
					So("authenticated reconnect", ShouldEqual, "timed out")
				}

				So(awaitChannels(3, "level3"), ShouldBeTrue)
				So(live.waitReady(), ShouldBeNil)
				So(live.Client() != client, ShouldBeTrue)
				So(tokenRequests.Load(), ShouldEqual, 3)
				So(live.Client().Token, ShouldEqual, "token-3")
				So(live.Error(), ShouldBeNil)
				So(live.Status(), ShouldEqual, runtime.READY)

				Convey("Retired frames and disconnect callbacks cannot corrupt the replacement", func() {
					client.OnReceived.Call(sdk.NewWebSocketMessage([]byte(`{"channel":"level3","type":"snapshot","data":[{"symbol":"BTC/USD","bids":[{"order_id":"retired","limit_price":123,"order_qty":1}],"asks":[]}]}`)))
					client.OnDisconnected.Call(errors.New("late retired socket failure"))
					live.book.Book("BTC/USD", func(managed *spotbook.Book) {
						So(managed.BestBid(), ShouldBeNil)
					})
					So(live.connected.Load(), ShouldBeTrue)
					So(live.Status(), ShouldEqual, runtime.READY)
					So(tokenRequests.Load(), ShouldEqual, 3)
				})
			})
		}
	})
}

func TestLiveSubTicker(t *testing.T) {
	Convey("Given an unconnected spot session", t, func() {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		live := &Live{
			ctx:    ctx,
			cancel: cancel,
			status: runtime.NewStatus(),
		}

		Convey("A ticker subscription should fail before writing to the socket", func() {
			live.SubTicker([]string{"BTC/USD"})

			So(live.Error(), ShouldNotBeNil)
			So(live.Status(), ShouldEqual, runtime.ERROR)
			So(live.ctx.Err(), ShouldEqual, context.Canceled)
		})
	})
}

func TestLiveSubTrades(t *testing.T) {
	Convey("Given an unconnected spot session", t, func() {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		live := &Live{
			ctx:    ctx,
			cancel: cancel,
			status: runtime.NewStatus(),
		}

		Convey("A trade subscription should fail before writing to the socket", func() {
			live.SubTrades([]string{"BTC/USD"})

			So(live.Error(), ShouldNotBeNil)
			So(live.Status(), ShouldEqual, runtime.ERROR)
			So(live.ctx.Err(), ShouldEqual, context.Canceled)
		})
	})
}

func TestLiveSubL3(t *testing.T) {
	Convey("Given an unconnected Level3 parent", t, func() {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		live := &Live{
			ctx:    ctx,
			cancel: cancel,
			status: runtime.NewStatus(),
		}

		Convey("A Level3 subscription should fail before creating a child", func() {
			live.SubL3([]string{"BTC/USD"})

			So(live.Error(), ShouldNotBeNil)
			So(live.Status(), ShouldEqual, runtime.ERROR)
			So(live.level3, ShouldBeNil)
		})
	})
}

func TestLiveAttachLevel3(t *testing.T) {
	Convey("Given an admitted Level3 parent and a newly connected child", t, func() {
		parentCtx, parentCancel := context.WithCancel(t.Context())
		childCtx, childCancel := context.WithCancel(t.Context())
		defer parentCancel()
		defer childCancel()
		parent := &Live{
			ctx:     parentCtx,
			cancel:  parentCancel,
			status:  runtime.NewStatus().Transition(runtime.BUSY).Transition(runtime.READY),
			ingress: readyTestIngress("level3"),
		}
		child := &Live{
			ctx:     childCtx,
			cancel:  childCancel,
			status:  runtime.NewStatus().Transition(runtime.BUSY),
			ingress: parent.ingress,
		}

		Convey("Attaching the child should release it before its snapshot subscription", func() {
			parent.AttachLevel3("BTC/USD", child)

			So(child.Status(), ShouldEqual, runtime.READY)
		})
	})
}

func TestLiveMarkReady(t *testing.T) {
	Convey("Given a waiting Level3 child already attached to its connected parent", t, func() {
		parentCtx, parentCancel := context.WithCancel(t.Context())
		childCtx, childCancel := context.WithCancel(t.Context())
		defer parentCancel()
		defer childCancel()
		parent := &Live{
			ctx:     parentCtx,
			cancel:  parentCancel,
			status:  runtime.NewStatus().Transition(runtime.BUSY),
			ingress: readyTestIngress("level3"),
		}
		child := &Live{
			ctx:     childCtx,
			cancel:  childCancel,
			status:  runtime.NewStatus().Transition(runtime.BUSY),
			ingress: parent.ingress,
		}
		parent.AttachLevel3("BTC/USD", child)

		Convey("Releasing the parent should release the existing child", func() {
			parent.MarkReady()

			So(parent.Status(), ShouldEqual, runtime.READY)
			So(child.Status(), ShouldEqual, runtime.READY)
		})
	})

	Convey("Given a connected spot session with a waiting consumer", t, func() {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		live := &Live{
			ctx:    ctx,
			cancel: cancel,
			status: runtime.NewStatus().Transition(runtime.BUSY),
			ingress: map[string]runtime.Ingress[*types.Envelope]{
				"ticker": &testIngress{
					status: runtime.NewStatus().Transition(runtime.WAITING),
				},
			},
		}

		Convey("Releasing the transport should fail the lifecycle", func() {
			live.MarkReady()

			So(live.Error(), ShouldNotBeNil)
			So(live.Status(), ShouldEqual, runtime.ERROR)
		})
	})
}

/* liveFixture owns the socket, resident book, numeric observers and notification sink. */
type liveFixture struct {
	live    *Live
	client  *spot.WebSocket
	ingress *testIngress
}

func newLiveFixture(t testing.TB) liveFixture {
	upgrader := gorillawebsocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		connection, err := upgrader.Upgrade(writer, request, nil)

		if err != nil {
			return
		}

		defer connection.Close()

		for {
			if _, _, err := connection.ReadMessage(); err != nil {
				return
			}
		}
	}))
	t.Cleanup(server.Close)
	client := spot.NewWebSocket()
	client.URL = "ws" + strings.TrimPrefix(server.URL, "http")
	client.REST.Executor = func(request *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header),
			Body: io.NopCloser(strings.NewReader(`{"error":[],"result":{}}`)), Request: request}, nil
	}
	ingress := &testIngress{status: runtime.NewStatus().Transition(runtime.READY), frames: make(chan *types.Envelope, 1)}
	live := NewWithClient(t.Context(), map[string]runtime.Ingress[*types.Envelope]{"level3": ingress},
		nil, false, system.Cfg.WebSocket.Endpoints.Level3, client)
	t.Cleanup(live.Close)
	if err := live.Error(); err != nil {
		t.Fatal(err)
	}
	live.MarkReady()
	live.level3Observers = []runtime.Node[*types.Envelope]{depthflow.NewSignal(t.Context()), morphology.NewSignal(t.Context())}
	return liveFixture{live: live, client: client, ingress: ingress}
}

func TestNewWithClient(t *testing.T) {
	Convey("Given a live Level3 transport with a resident book", t, func() {
		fixture := newLiveFixture(t)
		live, client, ingress := fixture.live, fixture.client, fixture.ingress

		Convey("BUSY seeds the book without releasing market observations", func() {
			live.status.Transition(runtime.BUSY)
			live.book.Expect([]string{"TEST/USD"})
			client.OnReceived.Call(sdk.NewWebSocketMessage([]byte(`{"channel":"level3","type":"snapshot","data":[{"symbol":"TEST/USD","timestamp":"2026-09-05T10:00:00Z","bids":[{"order_id":"bid","limit_price":100,"order_qty":3}],"asks":[{"order_id":"ask","limit_price":101,"order_qty":4}]}]}`)))
			So(live.book.Wait(), ShouldBeNil)
			So(live.Status(), ShouldEqual, runtime.BUSY)
			So(len(ingress.frames), ShouldEqual, 0)
			live.book.Book("TEST/USD", func(current *spotbook.Book) { So(current.BestBid().Price.String(), ShouldEqual, "100") })
			live.MarkReady()
			client.OnReceived.Call(sdk.NewWebSocketMessage([]byte(`{"channel":"level3","type":"update","data":[{"symbol":"TEST/USD","timestamp":"2026-09-05T10:00:01Z","bids":[{"event":"modify","order_id":"bid","limit_price":100,"order_qty":4}],"asks":[]}]}`)))
			So(live.Status(), ShouldEqual, runtime.READY)
			So(len(ingress.frames), ShouldEqual, 1)
		})

		Convey("Overlapping receive callbacks preserve whole-frame order and symbol identity", func() {
			// A replaced SDK session shares callbacks with its predecessor. Model
			// both receive goroutines delivering distinct, multi-symbol frames.
			const framesPerSession = 64
			var producers sync.WaitGroup
			producers.Add(2)

			for session := range 2 {
				go func() {
					defer producers.Done()

					for frame := range framesPerSession {
						payload := fmt.Sprintf(`{"channel":"level3","type":"update","data":[{"symbol":"SESSION%d-A/USD","timestamp":"2026-09-05T10:00:00Z","bids":[{"event":"add","order_id":"bid","limit_price":100,"order_qty":3}],"asks":[{"event":"add","order_id":"ask","limit_price":101,"order_qty":4}]},{"symbol":"SESSION%d-B/USD","timestamp":"2026-09-05T10:00:00Z","bids":[{"event":"add","order_id":"bid","limit_price":%d,"order_qty":3}],"asks":[{"event":"add","order_id":"ask","limit_price":201,"order_qty":4}]}]}`, session, session, 100+frame)
						client.OnReceived.Call(sdk.NewWebSocketMessage([]byte(payload)))
					}
				}()
			}

			for sequence := uint64(1); sequence <= framesPerSession*2; sequence++ {
				var firstSymbol string

				for ordinal := uint64(0); ordinal < 2; ordinal++ {
					select {
					case envelope := <-ingress.frames:
						So(envelope.Stream.Sequence, ShouldEqual, sequence)
						So(envelope.CaptureOrdinal, ShouldEqual, ordinal)
						So(envelope.DepthFlow, ShouldNotBeNil)
						So(envelope.Level3Data.Bids, ShouldBeNil)
						So(envelope.Level3Data.Asks, ShouldBeNil)

						if ordinal == 0 {
							firstSymbol = envelope.Level3Data.Symbol
							continue
						}
						So(envelope.Level3Data.Symbol, ShouldEqual, strings.Replace(firstSymbol, "-A/", "-B/", 1))
					case <-time.After(time.Second):
						t.Fatal("overlapping session callbacks stopped publishing")
					}
				}
			}
			producers.Wait()
			So(live.Error(), ShouldBeNil)
		})

		Convey("Snapshots, modifications and deletions should publish only book notifications", func() {
			for index, operation := range []string{"add", "modify", "delete"} {
				payload := fmt.Sprintf(`{"channel":"level3","type":"update","data":[{"symbol":"TEST/USD","timestamp":"2026-09-05T10:00:0%dZ","bids":[{"event":"%s","order_id":"bid","limit_price":100,"order_qty":3,"timestamp":"2026-09-05T10:00:0%dZ"}],"asks":[{"event":"add","order_id":"ask","limit_price":101,"order_qty":4,"timestamp":"2026-09-05T10:00:0%dZ"}]}]}`,
					index, operation, index, index)
				client.OnReceived.Call(sdk.NewWebSocketMessage([]byte(payload)))
				So(live.Error(), ShouldBeNil)

				select {
				case envelope := <-ingress.frames:
					So(envelope.TypeID, ShouldEqual, types.EnvelopeLevel3)
					So(envelope.Level3Data.Symbol, ShouldEqual, "TEST/USD")
					So(envelope.Level3Data.Timestamp.IsZero(), ShouldBeFalse)
					So(envelope.Level3Data.Bids, ShouldBeNil)
					So(envelope.Level3Data.Asks, ShouldBeNil)
					So(envelope.DepthFlow, ShouldNotBeNil)

					if operation != "delete" {
						So(envelope.Morphology, ShouldNotBeNil)
					}

					So(envelope.Stream.Sequence, ShouldEqual, index+1)
				case <-time.After(time.Second):
					t.Fatal("book update did not publish its notification")
				}

				live.book.Book("TEST/USD", func(book *spotbook.Book) {
					So(book.Asks.Low.Price.String(), ShouldEqual, "101")

					if operation == "delete" {
						So(book.Bids.High, ShouldBeNil)
						return
					}

					So(book.Bids.High.Price.String(), ShouldEqual, "100")
					So(book.Bids.High.Quantity.String(), ShouldEqual, "3")
				})

				select {
				case <-ingress.frames:
					t.Fatal("book update published a duplicate envelope")
				default:
				}
			}
		})
	})
}

func TestLiveResyncLevel3(t *testing.T) {
	Convey("Given a divergent symbol on an authenticated Level3 socket", t, func() {
		frames := make(chan map[string]any, 2)
		serverErrors := make(chan error, 1)
		upgrader := gorillawebsocket.Upgrader{}
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			connection, err := upgrader.Upgrade(writer, request, nil)
			if err != nil {
				serverErrors <- err
				return
			}
			defer connection.Close()
			for {
				_, raw, err := connection.ReadMessage()
				if err != nil {
					serverErrors <- err
					return
				}
				var frame map[string]any
				if err := json.Unmarshal(raw, &frame); err != nil {
					serverErrors <- err
					return
				}
				frames <- frame
			}
		}))
		defer server.Close()
		client := spot.NewWebSocket()
		client.URL = "ws" + strings.TrimPrefix(server.URL, "http")
		client.Token = "test-token"
		client.Reconnect = nil
		So(client.Connect(), ShouldBeNil)
		defer func() { So(client.Disconnect(), ShouldBeNil) }()
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		live := &Live{ctx: ctx, cancel: cancel, status: runtime.NewStatus()}
		live.client.Store(client)
		oldDepth := viper.GetInt("market.l3_depth")
		viper.Set("market.l3_depth", 100)
		defer viper.Set("market.l3_depth", oldDepth)
		live.resyncLevel3("FIL/USD")

		for _, method := range []string{"unsubscribe", "subscribe"} {
			select {
			case frame := <-frames:
				So(frame["method"], ShouldEqual, method)
				params := frame["params"].(map[string]any)
				So(params["symbol"], ShouldResemble, []any{"FIL/USD"})
				So(params["channel"], ShouldEqual, "level3")
				if method == "subscribe" {
					So(params["snapshot"], ShouldEqual, true)
					So(params["depth"], ShouldEqual, 100)
					So(params["token"], ShouldEqual, "test-token")
				}
			case err := <-serverErrors:
				So(err, ShouldBeNil)
			case <-time.After(5 * time.Second):
				So("recovery request", ShouldEqual, "timed out")
			}
		}
		So(live.Error(), ShouldBeNil)
	})
}

func BenchmarkNewWithClient(b *testing.B) {
	fixture := newLiveFixture(b)
	payload := []byte(`{"channel":"level3","type":"update","data":[{"symbol":"TEST/USD","timestamp":"2026-09-05T10:00:00Z","bids":[{"event":"add","order_id":"bid","limit_price":100,"order_qty":3}],"asks":[{"event":"add","order_id":"ask","limit_price":101,"order_qty":4}]}]}`)
	b.ReportAllocs()

	for b.Loop() {
		fixture.client.OnReceived.Call(sdk.NewWebSocketMessage(payload))
		<-fixture.ingress.frames
	}

	if err := fixture.live.Error(); err != nil {
		b.Fatal(err)
	}
}

func TestLiveBook(t *testing.T) {
	Convey("Only a ready L3 session exposes its books for valuation", t, func() {
		managed := newBookFixture(t, "BTC/USD", 2, 2)
		managed.Create("BTC/USD", 10)
		child := &Live{book: managed, status: runtime.NewStatus()}
		parent := &Live{level3: &sync.Map{}}
		parent.level3.Store("BTC/USD", child)

		for _, stage := range []runtime.Stage{runtime.READY, runtime.WAITING, runtime.BUSY, runtime.READY} {
			child.status.Transition(stage)
			calls := 0
			parent.Book("BTC/USD", func(book *spotbook.Book) {
				calls++
				So(book != nil, ShouldEqual, stage == runtime.READY)
			})
			So(calls, ShouldEqual, 1)
		}
	})
}

func BenchmarkLiveBind(b *testing.B) {
	client := spot.NewWebSocket()
	live := &Live{receive: func(*callback.Event[*sdk.WebSocketMessage]) {}}
	live.client.Store(client)
	live.bind(client)
	message := sdk.NewWebSocketMessage([]byte(`{"channel":"heartbeat"}`))
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		client.OnReceived.Call(message)
	}
}
