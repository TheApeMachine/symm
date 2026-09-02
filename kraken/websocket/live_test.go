package websocket

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	gorillawebsocket "github.com/gorilla/websocket"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/types"
)

/* testIngress exposes the runtime status a transport release must honor. */
type testIngress struct {
	status *runtime.Status
}

func (ingress *testIngress) Push(*types.Envelope) {}

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

				frames <- struct {
					connection int64
					payload    []byte
				}{connection: connectionID, payload: payload}
			}
		}))
		defer server.Close()

		endpoint := "ws" + strings.TrimPrefix(server.URL, "http")
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

		Convey("An unexpected disconnect should retry fresh authentication and restore Level3", func() {
			So(firstConnection.Close(), ShouldBeNil)

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
			So(live.Client(), ShouldNotEqual, client)
			So(tokenRequests.Load(), ShouldEqual, 3)
			So(live.Client().Token, ShouldEqual, "token-3")
			So(live.Error(), ShouldBeNil)
			So(live.Status(), ShouldEqual, runtime.READY)
		})
	})
}

func TestLiveSubTicker(t *testing.T) {
	Convey("Given a connected spot session whose consumers are not ready", t, func() {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		live := &Live{
			ctx:    ctx,
			cancel: cancel,
			status: runtime.NewStatus().Transition(runtime.BUSY),
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
	Convey("Given a connected spot session whose consumers are not ready", t, func() {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		live := &Live{
			ctx:    ctx,
			cancel: cancel,
			status: runtime.NewStatus().Transition(runtime.BUSY),
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
	Convey("Given a connected Level3 parent whose consumers are not ready", t, func() {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		live := &Live{
			ctx:    ctx,
			cancel: cancel,
			status: runtime.NewStatus().Transition(runtime.BUSY),
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
