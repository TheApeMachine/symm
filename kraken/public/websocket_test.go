package public

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bytedance/sonic"
	"github.com/fasthttp/websocket"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	balancefixtures "github.com/theapemachine/symm/tests/fixtures/balances"
)

func TestNewWebSocket(t *testing.T) {
	Convey("Given a public websocket context", t, func() {
		pool := qpool.NewQ[any](t.Context(), 1, 1, nil)
		tree := dmt.NewTree("")

		Convey("When creating a WebSocket", func() {
			ws := newTestWebSocket(t.Context(), pool, tree)
			defer ws.Close()

			Convey("Then it should initialize the websocket state", func() {
				_, ticker := ws.broadcasts.Load("ticker")
				_, subscriber := ws.subscribers.Load("kraken:public")

				So(ws.ctx, ShouldNotBeNil)
				So(ws.cancel, ShouldNotBeNil)
				So(ws.pool, ShouldEqual, pool)
				So(ws.tree, ShouldEqual, tree)
				So(ws.instrument, ShouldNotBeNil)
				So(ticker, ShouldBeTrue)
				So(subscriber, ShouldBeTrue)
			})
		})
	})
}

func TestOnMessage(t *testing.T) {
	Convey("Given a public websocket", t, func() {
		pool := qpool.NewQ[any](t.Context(), 1, 1, nil)
		ws := newTestWebSocket(t.Context(), pool, dmt.NewTree(""))
		defer ws.Close()

		payload := datura.Map[any]{
			"method": "subscribe",
			"params": datura.Map[any]{
				"channel": "ticker",
			},
		}.Marshal()

		Convey("When a kraken public message arrives before connection", func() {
			err := ws.onMessage(datura.Acquire(
				"test", datura.APPJSON,
			).WithDestination(
				"kraken:public",
			).WithPayload(
				payload,
			))

			Convey("Then it should reject the write", func() {
				So(err, ShouldNotBeNil)
			})
		})

		Convey("When a message is addressed elsewhere", func() {
			err := ws.onMessage(datura.Acquire(
				"test", datura.APPJSON,
			).WithDestination(
				"desk",
			).WithPayload(
				payload,
			))

			Convey("Then it should ignore the destination as an error", func() {
				So(err, ShouldNotBeNil)
			})
		})

		Convey("When a kraken public message arrives after connection", func() {
			received := make(chan []byte, 1)
			server := newWebSocketServer(t, func(conn *websocket.Conn, request *http.Request) {
				_, wire, err := conn.ReadMessage()
				if err != nil {
					t.Errorf("read websocket message failed: %v", err)
					return
				}
				received <- wire
			})
			defer server.Close()

			endpoint := EndpointType("ws" + strings.TrimPrefix(server.URL, "http"))
			ws.connectMaxDelay = 2
			So(ws.Connect(endpoint, 1), ShouldBeNil)

			err := ws.onMessage(datura.Acquire(
				"test", datura.APPJSON,
			).WithDestination(
				"kraken:public",
			).WithPayload(
				payload,
			))

			Convey("Then it should write the payload to the websocket", func() {
				So(err, ShouldBeNil)

				select {
				case wire := <-received:
					So(wire, ShouldResemble, payload)
				case <-time.After(time.Second):
					t.Fatal("timed out waiting for websocket payload")
				}
			})
		})
	})
}

func TestPaperPrivateOnMessage(t *testing.T) {
	Convey("Given a paper private websocket", t, func() {
		previousModel := viper.GetString("trading.model")
		previousAddr := viper.GetString("emulator.addr")
		viper.Set("trading.model", "paper")
		viper.Set("emulator.addr", freeListenAddr(t))
		defer viper.Set("trading.model", previousModel)
		defer viper.Set("emulator.addr", previousAddr)

		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()

		pool := qpool.NewQ[any](ctx, 1, 1, nil)
		tree := dmt.NewTree("")
		ws, emulator := startPaperPrivateEmulator(t, ctx, pool, tree)
		defer ws.Close()
		defer emulator.Close()

		payload := []byte(`{"method":"subscribe"}`)

		Convey("When a private message arrives through the websocket connection", func() {
			err := ws.onMessage(datura.Acquire(
				"test", datura.APPJSON,
			).WithDestination(
				"kraken:private",
			).WithRole(
				"balances",
			).WithPayload(
				payload,
			))

			Convey("Then it should write through the same private websocket path", func() {
				So(err, ShouldBeNil)
			})
		})

		Convey("When a private order arrives through the websocket connection", func() {
			err := ws.onMessage(datura.Acquire(
				"test", datura.APPJSON,
			).WithDestination(
				"kraken:private",
			).WithRole(
				"orders",
			).WithPayload(
				[]byte(`{"method":"add_order","params":{"symbol":"BTC/USD","side":"buy","order_type":"market","order_qty":1,"cl_ord_id":"test"}}`),
			))

			Convey("Then it should write through the same private websocket path", func() {
				So(err, ShouldBeNil)
			})
		})
	})
}

func TestPaperPrivateBalanceSubscribeBroadcasts(t *testing.T) {
	Convey("Given a paper private websocket and balances subscriber", t, func() {
		previousModel := viper.GetString("trading.model")
		previousQuote := viper.GetString("market.quote_currency")
		previousWallet := viper.GetFloat64("trading.paper.wallet.usd")
		previousAddr := viper.GetString("emulator.addr")
		viper.Set("trading.model", "paper")
		viper.Set("market.quote_currency", "USD")
		viper.Set("trading.paper.wallet.usd", 200.0)
		viper.Set("emulator.addr", freeListenAddr(t))
		defer viper.Set("trading.model", previousModel)
		defer viper.Set("market.quote_currency", previousQuote)
		defer viper.Set("trading.paper.wallet.usd", previousWallet)
		defer viper.Set("emulator.addr", previousAddr)

		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()

		pool := qpool.NewQ[any](ctx, 1, 1, nil)
		tree := dmt.NewTree("")
		ws, emulator := startPaperPrivateEmulator(t, ctx, pool, tree)
		defer ws.Close()
		defer emulator.Close()

		balances := pool.Subscribe("balances", nil)

		Convey("When a private balances subscribe artifact arrives", func() {
			err := ws.onMessage(datura.Acquire(
				"test", datura.APPJSON,
			).WithDestination(
				"kraken:private",
			).WithRole(
				"balances",
			).WithPayload(
				[]byte(`{"method":"subscribe","params":{"channel":"balances"}}`),
			))

			Convey("Then the local handler should publish the same balance stream", func() {
				So(err, ShouldBeNil)

				waitCtx, waitCancel := context.WithTimeout(ctx, time.Second)
				defer waitCancel()

				artifact, waitErr := balances.Wait(waitCtx)
				So(waitErr, ShouldBeNil)
				So(artifact, ShouldNotBeNil)

				role, roleErr := artifact.Role()
				scope, scopeErr := artifact.Scope()
				So(roleErr, ShouldBeNil)
				So(scopeErr, ShouldBeNil)
				So(role, ShouldEqual, "balances")
				So(scope, ShouldEqual, "snapshot")
				So(balanceFrameValue(artifact, "channel"), ShouldEqual, "balances")
				So(balanceFrameValue(artifact, "type"), ShouldEqual, "snapshot")
				So(balanceFrameValue(artifact, "data", 0, "balance"), ShouldEqual, 200.0)
			})
		})
	})
}

func TestPaperPrivateBalanceFixturesThroughWebSocket(t *testing.T) {
	Convey("Given a paper private websocket and balance fixture sequence", t, func() {
		previousModel := viper.GetString("trading.model")
		previousQuote := viper.GetString("market.quote_currency")
		previousWallet := viper.GetFloat64("trading.paper.wallet.usd")
		viper.Set("trading.model", "paper")
		viper.Set("market.quote_currency", "USD")
		viper.Set("trading.paper.wallet.usd", 200.0)
		defer viper.Set("trading.model", previousModel)
		defer viper.Set("market.quote_currency", previousQuote)
		defer viper.Set("trading.paper.wallet.usd", previousWallet)

		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()

		pool := qpool.NewQ[any](ctx, 1, 1, nil)
		ws := newPrivateTestWebSocket(ctx, pool, dmt.NewTree(""))
		ws.connectMaxDelay = 2
		defer ws.Close()

		balances := pool.Subscribe("balances", nil)
		frames := balanceFixtureFrames(3)

		server := newWebSocketServer(t, func(conn *websocket.Conn, request *http.Request) {
			for _, frame := range frames {
				if err := conn.WriteMessage(websocket.TextMessage, frame); err != nil {
					t.Errorf("write balances fixture failed: %v", err)
					return
				}
			}

			<-request.Context().Done()
		})
		defer server.Close()

		Convey("When Run reads the snapshot and update frames", func() {
			endpoint := EndpointType("ws" + strings.TrimPrefix(server.URL, "http"))
			go ws.Run(endpoint)

			previousSequence := float64(0)

			for i := range frames {
				waitCtx, waitCancel := context.WithTimeout(ctx, time.Second)
				artifact, waitErr := balances.Wait(waitCtx)
				waitCancel()

				So(waitErr, ShouldBeNil)
				So(artifact, ShouldNotBeNil)

				role, roleErr := artifact.Role()
				scope, scopeErr := artifact.Scope()
				So(roleErr, ShouldBeNil)
				So(scopeErr, ShouldBeNil)
				So(role, ShouldEqual, "balances")
				So(scope, ShouldNotBeEmpty)
				So(balanceFrameValue(artifact, "channel"), ShouldEqual, "balances")

				if i == 0 {
					So(balanceFrameValue(artifact, "type"), ShouldEqual, "snapshot")
					So(len(balanceFrameValue(artifact, "data").([]any)), ShouldEqual, 3)
					continue
				}

				sequence := balanceFrameValue(artifact, "sequence").(float64)
				So(balanceFrameValue(artifact, "type"), ShouldEqual, "update")
				So(sequence, ShouldBeGreaterThan, previousSequence)
				So(balanceFrameValue(artifact, "data", 0, "balance"), ShouldNotBeNil)
				previousSequence = sequence
			}
		})
	})
}

func TestRun(t *testing.T) {
	Convey("Given a public websocket and a Kraken ticker frame", t, func() {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()

		pool := qpool.NewQ[any](ctx, 1, 1, nil)
		tree := dmt.NewTree("")
		ws := newTestWebSocket(ctx, pool, tree)
		ws.connectMaxDelay = 2
		defer ws.Close()

		server := newWebSocketServer(t, func(conn *websocket.Conn, request *http.Request) {
			if err := conn.WriteMessage(websocket.TextMessage, []byte(
				`{"channel":"status","type":"update","data":[{"version":"2"}]}`,
			)); err != nil {
				t.Errorf("write status frame failed: %v", err)
				return
			}

			if err := conn.WriteMessage(websocket.TextMessage, []byte(
				`{"channel":"ticker","type":"update","data":[{"symbol":"DOGE/USD","last":0.2}]}`,
			)); err != nil {
				t.Errorf("write ticker frame failed: %v", err)
				return
			}
			<-request.Context().Done()
		})
		defer server.Close()

		Convey("When Run reads the frame", func() {
			endpoint := EndpointType("ws" + strings.TrimPrefix(server.URL, "http"))
			go ws.Run(endpoint)

			artifact := waitForArtifact(t, tree, []byte("ticker/"))

			Convey("Then it should persist it by role, scope, and timestamp", func() {
				So(artifact, ShouldNotBeNil)
				So(datura.Peek[string](artifact, "role"), ShouldEqual, "ticker")
				So(datura.Peek[string](artifact, "scope"), ShouldEqual, "DOGE/USD")
				So(datura.Peek[string](artifact, "channel"), ShouldEqual, "ticker")
			})
		})
	})

	Convey("Given a public websocket and a multi-symbol Kraken ticker frame", t, func() {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()

		pool := qpool.NewQ[any](ctx, 1, 1, nil)
		tree := dmt.NewTree("")
		ws := newTestWebSocket(ctx, pool, tree)
		ws.connectMaxDelay = 2
		defer ws.Close()

		server := newWebSocketServer(t, func(conn *websocket.Conn, request *http.Request) {
			if err := conn.WriteMessage(websocket.TextMessage, []byte(
				`{"channel":"ticker","type":"update","data":[{"symbol":"DOGE/USD","last":0.2},{"symbol":"ETH/USD","last":2000}]}`,
			)); err != nil {
				t.Errorf("write ticker frame failed: %v", err)
				return
			}
			<-request.Context().Done()
		})
		defer server.Close()

		Convey("When Run reads the frame", func() {
			endpoint := EndpointType("ws" + strings.TrimPrefix(server.URL, "http"))
			go ws.Run(endpoint)

			artifacts := waitForScopedArtifacts(t, tree, []byte("ticker/"), "DOGE/USD", "ETH/USD")

			Convey("Then it should persist one artifact per symbol scope", func() {
				So(datura.Peek[string](artifacts["DOGE/USD"], "data", 0, "symbol"), ShouldEqual, "DOGE/USD")
				So(datura.Peek[string](artifacts["ETH/USD"], "data", 0, "symbol"), ShouldEqual, "ETH/USD")
			})
		})
	})

	Convey("Given a public websocket whose peer closes the connection", t, func() {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()

		pool := qpool.NewQ[any](ctx, 1, 1, nil)
		ws := newTestWebSocket(ctx, pool, dmt.NewTree(""))
		ws.connectMaxDelay = 2
		defer ws.Close()

		accepted := make(chan struct{}, 1)
		server := newWebSocketServer(t, func(conn *websocket.Conn, request *http.Request) {
			accepted <- struct{}{}
			errnie.Error(conn.Close())
		})

		Convey("When Run reads after the peer closes", func() {
			endpoint := EndpointType("ws" + strings.TrimPrefix(server.URL, "http"))
			go ws.Run(endpoint)

			select {
			case <-accepted:
				server.Close()
			case <-time.After(time.Second):
				t.Fatal("timed out waiting for websocket connection")
			}

			err := waitForWebSocketError(t, ws)
			cancel()

			Convey("Then it should drop the failed connection before the next read", func() {
				_, connected := ws.connection()
				So(err, ShouldNotBeNil)
				So(connected, ShouldBeFalse)
			})
		})
	})
}

func TestRunSubscribesInstrumentAfterConnect(t *testing.T) {
	Convey("Given a public websocket endpoint", t, func() {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()

		pool := qpool.NewQ[any](ctx, 1, 1, nil)
		ws := newTestWebSocket(ctx, pool, dmt.NewTree(""))
		ws.connectMaxDelay = 2
		defer ws.Close()

		received := make(chan []byte, 1)
		server := newWebSocketServer(t, func(conn *websocket.Conn, request *http.Request) {
			_, wire, err := conn.ReadMessage()
			if err != nil {
				t.Errorf("read websocket message failed: %v", err)
				return
			}

			received <- wire
			<-request.Context().Done()
		})
		defer server.Close()

		Convey("When Run connects", func() {
			endpoint := EndpointType("ws" + strings.TrimPrefix(server.URL, "http"))
			go ws.Run(endpoint)

			Convey("Then it should subscribe to the instrument channel", func() {
				select {
				case wire := <-received:
					So(string(wire), ShouldContainSubstring, `"method":"subscribe"`)
					So(string(wire), ShouldContainSubstring, `"channel":"instrument"`)
				case <-time.After(time.Second):
					t.Fatal("timed out waiting for instrument subscription")
				}
			})
		})
	})
}

func TestPaperPrivateRunDialsEmulator(t *testing.T) {
	Convey("Given a paper private websocket endpoint", t, func() {
		previousModel := viper.GetString("trading.model")
		viper.Set("trading.model", "paper")
		defer viper.Set("trading.model", previousModel)

		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()

		pool := qpool.NewQ[any](ctx, 1, 1, nil)
		ws := newPrivateTestWebSocket(ctx, pool, dmt.NewTree(""))
		ws.connectMaxDelay = 2
		defer ws.Close()

		var dialed atomic.Bool
		server := newWebSocketServer(t, func(conn *websocket.Conn, request *http.Request) {
			dialed.Store(true)
			<-request.Context().Done()
		})
		defer server.Close()

		Convey("When Run starts", func() {
			endpoint := EndpointType("ws" + strings.TrimPrefix(server.URL, "http"))
			go ws.Run(endpoint)
			time.Sleep(50 * time.Millisecond)

			Convey("Then it should open the local websocket emulator", func() {
				So(dialed.Load(), ShouldBeTrue)
			})
		})
	})
}

func TestPrivateRunSubscribesAccountChannelsAfterConnect(t *testing.T) {
	Convey("Given a private websocket endpoint", t, func() {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()

		pool := qpool.NewQ[any](ctx, 1, 1, nil)
		ws := newPrivateTestWebSocket(ctx, pool, dmt.NewTree(""))
		ws.connectMaxDelay = 2
		defer ws.Close()

		received := make(chan string, 2)
		server := newWebSocketServer(t, func(conn *websocket.Conn, request *http.Request) {
			for index := 0; index < 2; index++ {
				_, wire, err := conn.ReadMessage()
				if err != nil {
					t.Errorf("read websocket message failed: %v", err)
					return
				}

				received <- string(wire)
			}

			<-request.Context().Done()
		})
		defer server.Close()

		Convey("When Run connects", func() {
			endpoint := EndpointType("ws" + strings.TrimPrefix(server.URL, "http"))
			go ws.Run(endpoint)

			Convey("Then it should subscribe to balances and executions", func() {
				channels := map[string]bool{}

				for len(channels) < 2 {
					select {
					case wire := <-received:
						So(wire, ShouldContainSubstring, `"method":"subscribe"`)
						if strings.Contains(wire, `"channel":"balances"`) {
							channels["balances"] = true
						}
						if strings.Contains(wire, `"channel":"executions"`) {
							channels["executions"] = true
						}
					case <-time.After(time.Second):
						t.Fatal("timed out waiting for private account subscriptions")
					}
				}

				So(channels["balances"], ShouldBeTrue)
				So(channels["executions"], ShouldBeTrue)
			})
		})
	})
}

func TestError(t *testing.T) {
	Convey("Given a public websocket with an error", t, func() {
		ws := newTestWebSocket(t.Context(), qpool.NewQ[any](t.Context(), 1, 1, nil), dmt.NewTree(""))
		defer ws.Close()
		expected := errnie.Err(errnie.Unknown, "test", nil)
		ws.setError(expected)

		Convey("When Error is called", func() {
			err := ws.Error()

			Convey("Then it should return the stored error", func() {
				So(err, ShouldEqual, expected)
			})
		})
	})
}

func TestClose(t *testing.T) {
	Convey("Given a connected public websocket", t, func() {
		pool := qpool.NewQ[any](t.Context(), 1, 1, nil)
		ws := newTestWebSocket(t.Context(), pool, dmt.NewTree(""))

		server := newWebSocketServer(t, func(conn *websocket.Conn, request *http.Request) {
			<-request.Context().Done()
		})
		defer server.Close()

		endpoint := EndpointType("ws" + strings.TrimPrefix(server.URL, "http"))
		ws.connectMaxDelay = 2
		So(ws.Connect(endpoint, 1), ShouldBeNil)

		Convey("When Close is called", func() {
			err := ws.Close()

			Convey("Then it should close and clear the connection", func() {
				_, connected := ws.connection()
				So(err, ShouldBeNil)
				So(connected, ShouldBeFalse)
			})
		})
	})
}

func TestConnect(t *testing.T) {
	Convey("Given a public websocket endpoint", t, func() {
		pool := qpool.NewQ[any](t.Context(), 1, 1, nil)
		ws := newTestWebSocket(t.Context(), pool, dmt.NewTree(""))
		defer ws.Close()

		server := newWebSocketServer(t, func(conn *websocket.Conn, request *http.Request) {
			<-request.Context().Done()
		})
		defer server.Close()

		Convey("When Connect dials a websocket server", func() {
			endpoint := EndpointType("ws" + strings.TrimPrefix(server.URL, "http"))
			ws.connectMaxDelay = 2
			err := ws.Connect(endpoint, 1)

			Convey("Then it should mark the websocket connected", func() {
				conn, connected := ws.connection()
				So(err, ShouldBeNil)
				So(conn, ShouldNotBeNil)
				So(connected, ShouldBeTrue)
			})
		})

		Convey("When the attempt exceeds the max delay", func() {
			ws.connectMaxDelay = 0
			err := ws.Connect(WebSocketURL, 1)

			Convey("Then it should stop before dialing", func() {
				So(err, ShouldNotBeNil)
				So(ws.isConnected.Load(), ShouldBeFalse)
			})
		})
	})
}

func TestDisconnect(t *testing.T) {
	Convey("Given a connected public websocket", t, func() {
		pool := qpool.NewQ[any](t.Context(), 1, 1, nil)
		ws := newTestWebSocket(t.Context(), pool, dmt.NewTree(""))
		defer ws.Close()

		server := newWebSocketServer(t, func(conn *websocket.Conn, request *http.Request) {
			<-request.Context().Done()
		})
		defer server.Close()

		endpoint := EndpointType("ws" + strings.TrimPrefix(server.URL, "http"))
		ws.connectMaxDelay = 2
		So(ws.Connect(endpoint, 1), ShouldBeNil)

		Convey("When disconnect is called", func() {
			ws.disconnect()

			Convey("Then it should clear connection state", func() {
				_, connected := ws.connection()
				So(connected, ShouldBeFalse)
			})
		})
	})
}

func newTestWebSocket(
	ctx context.Context,
	pool *qpool.Q[any],
	tree *dmt.Tree,
) *WebSocket {
	return NewWebSocket(
		ctx,
		pool,
		tree,
		nil,
		[]string{"ticker"},
		[]string{"kraken:public"},
	)
}

func newPrivateTestWebSocket(
	ctx context.Context,
	pool *qpool.Q[any],
	tree *dmt.Tree,
) *WebSocket {
	return NewWebSocket(
		ctx,
		pool,
		tree,
		nil,
		[]string{"balances", "executions", "orders"},
		[]string{"kraken:private"},
	)
}

func startPaperPrivateEmulator(
	t *testing.T,
	ctx context.Context,
	pool *qpool.Q[any],
	tree *dmt.Tree,
) (*WebSocket, *Emulator) {
	t.Helper()

	emulator, err := NewEmulator(ctx, pool, tree)
	So(err, ShouldBeNil)

	go func() {
		errnie.Error(emulator.Serve())
	}()

	ws := newPrivateTestWebSocket(ctx, pool, tree)
	ws.connectMaxDelay = 2

	go ws.Run(emulator.Endpoint())

	waitForConnected(t, ws)

	return ws, emulator
}

func freeListenAddr(t *testing.T) string {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	So(err, ShouldBeNil)
	defer listener.Close()

	return listener.Addr().String()
}

func balanceFixtureFrames(updateCount int) [][]byte {
	frames := make([][]byte, 0, updateCount+1)

	for payload := range balancefixtures.NewFixture(balancefixtures.SNAPSHOT, 1).Generate() {
		frames = append(frames, payload)
	}

	for payload := range balancefixtures.NewFixture(balancefixtures.UPDATE, updateCount).Generate() {
		frames = append(frames, payload)
	}

	return frames
}

func balanceFrameValue(artifact *datura.Artifact, path ...any) any {
	var payload map[string]any

	if err := sonic.Unmarshal(artifact.DecryptPayload(), &payload); err != nil {
		return nil
	}

	current := any(payload)

	for _, segment := range path {
		switch typed := current.(type) {
		case map[string]any:
			key, _ := segment.(string)
			current = typed[key]
		case []any:
			index, _ := segment.(int)
			if index < 0 || index >= len(typed) {
				return nil
			}
			current = typed[index]
		default:
			return nil
		}
	}

	return current
}

func newWebSocketServer(
	t *testing.T,
	handle func(*websocket.Conn, *http.Request),
) *httptest.Server {
	t.Helper()

	upgrader := websocket.Upgrader{
		CheckOrigin: func(request *http.Request) bool {
			return true
		},
	}

	return httptest.NewServer(http.HandlerFunc(
		func(response http.ResponseWriter, request *http.Request) {
			conn, err := upgrader.Upgrade(response, request, nil)
			if err != nil {
				t.Errorf("upgrade failed: %v", err)
				return
			}
			defer conn.Close()

			handle(conn, request)
		},
	))
}

func waitForArtifact(
	t *testing.T,
	tree *dmt.Tree,
	prefix []byte,
) *datura.Artifact {
	t.Helper()

	deadline := time.Now().Add(3 * time.Second)

	for time.Now().Before(deadline) {
		for artifact := range tree.Seek(prefix) {
			return artifact
		}

		time.Sleep(time.Millisecond)
	}

	t.Fatalf("timed out waiting for artifact under %q", string(prefix))
	return nil
}

func waitForScopedArtifacts(
	t *testing.T,
	tree *dmt.Tree,
	prefix []byte,
	scopes ...string,
) map[string]*datura.Artifact {
	t.Helper()

	want := make(map[string]struct{}, len(scopes))
	for _, scope := range scopes {
		want[scope] = struct{}{}
	}

	found := make(map[string]*datura.Artifact, len(scopes))
	deadline := time.Now().Add(3 * time.Second)

	for time.Now().Before(deadline) {
		for artifact := range tree.Seek(prefix) {
			scope, _ := artifact.Scope()

			if _, ok := want[scope]; ok {
				found[scope] = artifact
			}
		}

		if len(found) == len(want) {
			return found
		}

		time.Sleep(time.Millisecond)
	}

	t.Fatalf("timed out waiting for scopes %v under %q", scopes, string(prefix))
	return nil
}

func waitForWebSocketError(t *testing.T, ws *WebSocket) error {
	t.Helper()

	deadline := time.Now().Add(3 * time.Second)

	for time.Now().Before(deadline) {
		_, connected := ws.connection()
		if ws.Error() != nil && !connected {
			return ws.Error()
		}

		time.Sleep(time.Millisecond)
	}

	t.Fatal("timed out waiting for websocket read error")
	return nil
}

func waitForConnected(t *testing.T, ws *WebSocket) {
	t.Helper()

	deadline := time.Now().Add(3 * time.Second)

	for time.Now().Before(deadline) {
		if _, connected := ws.connection(); connected {
			return
		}

		time.Sleep(time.Millisecond)
	}

	t.Fatal("timed out waiting for websocket connection")
}
