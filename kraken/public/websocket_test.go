package public

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fasthttp/websocket"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/qpool"
)

const websocketPollInterval = 10 * time.Millisecond

var krakenBroadcastChannels = []string{
	"ohlc", "instrument", "ticker", "book", "trade", "execution", "status",
}

func websocketTestTree(t testing.TB) *dmt.Tree {
	if t != nil {
		t.Helper()
	}

	return dmt.NewTree("")
}

func websocketTestPool(t testing.TB) *qpool.Q[any] {
	if t != nil {
		t.Helper()
	}

	pool := qpool.NewQ[any](t.Context(), 1, 2, nil)

	if pool == nil && t != nil {
		t.Fatal("qpool.NewQ returned nil")
	}

	return pool
}

func testWebSocketEndpoint(
	t testing.TB,
	handler func(*websocket.Conn),
) (EndpointType, func()) {
	t.Helper()

	upgrader := websocket.Upgrader{
		CheckOrigin: func(*http.Request) bool {
			return true
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(
		writer http.ResponseWriter,
		request *http.Request,
	) {
		conn, upgradeErr := upgrader.Upgrade(writer, request, nil)

		if upgradeErr != nil {
			return
		}

		handler(conn)
	}))

	endpoint := EndpointType(
		"ws" + strings.TrimPrefix(server.URL, "http"),
	)

	return endpoint, server.Close
}

func waitUntil(timeout time.Duration, condition func() bool) bool {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		if condition() {
			return true
		}

		time.Sleep(websocketPollInterval)
	}

	return condition()
}

func treeContainsFrame(tree *dmt.Tree, prefix string, frame []byte) bool {
	if tree == nil || len(frame) == 0 {
		return false
	}

	for inbound := range tree.Seek([]byte(prefix)) {
		payload := inbound.DecryptPayload()
		inbound.Release()

		if len(payload) > 0 && string(payload) == string(frame) {
			return true
		}
	}

	return false
}

func holdServerConn(conn *websocket.Conn) {
	time.Sleep(200 * time.Millisecond)
	_ = conn.Close()
}

func WithConnectedWebSocket(
	t testing.TB,
	ws *WebSocket,
	handler func(*websocket.Conn),
	block func(),
) func() {
	return func() {
		endpoint, cleanup := testWebSocketEndpoint(t, handler)

		Reset(cleanup)
		So(ws.Connect(endpoint, 1), ShouldBeNil)

		block()
	}
}

func WithRunningWebSocket(
	t testing.TB,
	pool *qpool.Q[any],
	tree *dmt.Tree,
	handler func(*websocket.Conn),
	block func(ws *WebSocket),
) func() {
	return func() {
		endpoint, cleanup := testWebSocketEndpoint(t, handler)
		runCtx, cancelRun := context.WithCancel(t.Context())
		runnable := NewWebSocket(runCtx, pool, tree)

		go runnable.Run(endpoint)

		Reset(func() {
			cancelRun()
			cleanup()
		})

		block(runnable)
	}
}

func WithKrakenArtifact(
	destination string,
	payload []byte,
	block func(artifact *datura.Artifact),
) func() {
	return func() {
		artifact := datura.Acquire(
			"kraken:public", datura.Artifact_Type_json,
		).WithDestination(destination)

		if len(payload) > 0 {
			artifact.WithPayload(payload)
		}

		Reset(func() {
			artifact.Release()
		})

		block(artifact)
	}
}

func TestWebSocketRun(t *testing.T) {
	viper.Set("system.network.connection.max_delay", 89)

	Convey("Given a public websocket", t, func() {
		pool := websocketTestPool(t)
		tree := websocketTestTree(t)
		ws := NewWebSocket(t.Context(), pool, tree)

		So(ws, ShouldNotBeNil)
		So(ws.tree, ShouldEqual, tree)

		for _, channel := range krakenBroadcastChannels {
			_, broadcastOK := ws.broadcasts.Load(channel)
			So(broadcastOK, ShouldBeTrue)
		}

		_, subscriberOK := ws.subscribers.Load("kraken:public")
		So(subscriberOK, ShouldBeTrue)

		Convey("When the websocket is run", WithRunningWebSocket(t, pool, tree, func(conn *websocket.Conn) {
			time.Sleep(2 * time.Second)
			_ = conn.Close()
		}, func(runnable *WebSocket) {
			Convey("It should connect to the websocket", func() {
				So(waitUntil(2*time.Second, runnable.isConnected.Load), ShouldBeTrue)
			})

			Convey("It should reconnect after the server closes the connection", func() {
				var dialCount atomic.Int32

				endpoint, cleanup := testWebSocketEndpoint(t, func(conn *websocket.Conn) {
					count := dialCount.Add(1)

					if count == 1 {
						_ = conn.Close()
						return
					}

					time.Sleep(5 * time.Second)
				})

				runCtx, cancelRun := context.WithCancel(t.Context())
				reconnectable := NewWebSocket(runCtx, pool, tree)

				go reconnectable.Run(endpoint)

				Reset(func() {
					cancelRun()
					cleanup()
				})

				So(waitUntil(5*time.Second, func() bool {
					return dialCount.Load() >= 2
				}), ShouldBeTrue)
				So(waitUntil(5*time.Second, reconnectable.isConnected.Load), ShouldBeTrue)
			})
		}))

		Convey("When a message is received", WithRunningWebSocket(t, pool, tree, func(conn *websocket.Conn) {
			frame := []byte(`{"channel":"heartbeat","type":"update"}`)
			_ = conn.WriteMessage(websocket.TextMessage, frame)
			time.Sleep(200 * time.Millisecond)
			_ = conn.Close()
		}, func(runnable *WebSocket) {
			frame := []byte(`{"channel":"heartbeat","type":"update"}`)

			So(waitUntil(2*time.Second, runnable.isConnected.Load), ShouldBeTrue)
			So(waitUntil(2*time.Second, func() bool {
				return treeContainsFrame(runnable.tree, "heartbeat/", frame)
			}), ShouldBeTrue)
		}))

		Convey("When a book message is received", WithRunningWebSocket(t, pool, tree, func(conn *websocket.Conn) {
			frame := []byte(`{"channel":"book","type":"update","data":[{"symbol":"BTC/USD","bids":[["50000.0","1.0"]]}]}`)
			_ = conn.WriteMessage(websocket.TextMessage, frame)
			time.Sleep(200 * time.Millisecond)
			_ = conn.Close()
		}, func(runnable *WebSocket) {
			frame := []byte(`{"channel":"book","type":"update","data":[{"symbol":"BTC/USD","bids":[["50000.0","1.0"]]}]}`)

			So(waitUntil(2*time.Second, runnable.isConnected.Load), ShouldBeTrue)
			So(waitUntil(2*time.Second, func() bool {
				return treeContainsFrame(runnable.tree, "book/BTC/USD", frame)
			}), ShouldBeTrue)
		}))

		Convey("When a trade message is received", WithRunningWebSocket(t, pool, tree, func(conn *websocket.Conn) {
			frame := []byte(`{"channel":"trade","type":"update","data":[{"symbol":"BTC/USD","price":"50000.0","qty":"0.1","side":"buy"}]}`)
			_ = conn.WriteMessage(websocket.TextMessage, frame)
			time.Sleep(200 * time.Millisecond)
			_ = conn.Close()
		}, func(runnable *WebSocket) {
			frame := []byte(`{"channel":"trade","type":"update","data":[{"symbol":"BTC/USD","price":"50000.0","qty":"0.1","side":"buy"}]}`)

			So(waitUntil(2*time.Second, runnable.isConnected.Load), ShouldBeTrue)
			So(waitUntil(2*time.Second, func() bool {
				return treeContainsFrame(runnable.tree, "trade/BTC/USD", frame)
			}), ShouldBeTrue)
		}))

		Convey("When a ticker message is received", WithRunningWebSocket(t, pool, tree, func(conn *websocket.Conn) {
			frame := []byte(`{"channel":"ticker","type":"update","data":[{"symbol":"BTC/USD","last":"50000.0"}]}`)
			_ = conn.WriteMessage(websocket.TextMessage, frame)
			time.Sleep(200 * time.Millisecond)
			_ = conn.Close()
		}, func(runnable *WebSocket) {
			frame := []byte(`{"channel":"ticker","type":"update","data":[{"symbol":"BTC/USD","last":"50000.0"}]}`)

			So(waitUntil(2*time.Second, runnable.isConnected.Load), ShouldBeTrue)
			So(waitUntil(2*time.Second, func() bool {
				return treeContainsFrame(runnable.tree, "ticker/BTC/USD", frame)
			}), ShouldBeTrue)
		}))

		Convey("When the run context is cancelled", func() {
			endpoint, cleanup := testWebSocketEndpoint(t, func(conn *websocket.Conn) {
				frame := []byte(`{"channel":"heartbeat","type":"update"}`)
				_ = conn.WriteMessage(websocket.TextMessage, frame)
				_ = conn.Close()
			})

			runCtx, cancelRun := context.WithCancel(t.Context())
			runnable := NewWebSocket(runCtx, pool, tree)
			runDone := make(chan struct{})

			go func() {
				runnable.Run(endpoint)
				close(runDone)
			}()

			Reset(func() {
				cancelRun()
				cleanup()
			})

			frame := []byte(`{"channel":"heartbeat","type":"update"}`)

			So(waitUntil(2*time.Second, runnable.isConnected.Load), ShouldBeTrue)
			So(waitUntil(2*time.Second, func() bool {
				return treeContainsFrame(runnable.tree, "heartbeat/", frame)
			}), ShouldBeTrue)

			cancelRun()

			So(waitUntil(2*time.Second, func() bool {
				select {
				case <-runDone:
					return true
				default:
					return false
				}
			}), ShouldBeTrue)
		})

		Convey("When a message is received with an invalid destination", WithConnectedWebSocket(t, ws, holdServerConn, func() {
			Convey("It should return an error", WithKrakenArtifact("invalid", nil, func(artifact *datura.Artifact) {
				So(ws.onMessage(artifact), ShouldNotBeNil)
			}))
		}))

		Convey("When a message is received with an invalid payload", WithConnectedWebSocket(t, ws, holdServerConn, func() {
			Convey("It should not return an error", WithKrakenArtifact("kraken:public", nil, func(artifact *datura.Artifact) {
				So(ws.onMessage(artifact), ShouldBeNil)
			}))
		}))

		Convey("When a message is received from the kraken:public channel", func() {
			payload := []byte(`{"method":"subscribe","params":{"channel":"ticker"}}`)
			received := make(chan []byte, 1)

			Convey("It should write the payload to the websocket", WithConnectedWebSocket(t, ws, func(conn *websocket.Conn) {
				messageType, message, readErr := conn.ReadMessage()

				if readErr == nil && messageType == websocket.TextMessage {
					received <- append([]byte(nil), message...)
				}

				holdServerConn(conn)
			}, func() {
				Convey("for the subscribed payload", WithKrakenArtifact("kraken:public", payload, func(artifact *datura.Artifact) {
					So(ws.onMessage(artifact), ShouldBeNil)

					var wire []byte

					select {
					case wire = <-received:
					case <-time.After(2 * time.Second):
						So("kraken:public payload", ShouldEqual, "written")
					}

					So(string(wire), ShouldEqual, string(payload))
				}))
			}))
		})

		Convey("When onMessage writes after the connection is closed", WithConnectedWebSocket(t, ws, func(conn *websocket.Conn) {
			time.Sleep(100 * time.Millisecond)
		}, func() {
			payload := []byte(`{"method":"ping"}`)

			_ = ws.conn.Close()

			Convey("It should not return an error", WithKrakenArtifact("kraken:public", payload, func(artifact *datura.Artifact) {
				So(ws.onMessage(artifact), ShouldBeNil)
			}))
		}))

		Convey("When the websocket is closed", WithConnectedWebSocket(t, ws, func(conn *websocket.Conn) {
			time.Sleep(200 * time.Millisecond)
		}, func() {
			Convey("It should close the connection", func() {
				So(ws.Close(), ShouldBeNil)
				So(ws.conn.Close(), ShouldNotBeNil)
			})
		}))
	})
}

func TestWebSocketSubscribeRetryOnFailure(t *testing.T) {
	viper.Set("system.network.connection.max_delay", 89)
	viper.Set("market.subscribe_pace", 0)
	viper.Set("market.subscribe_batch", 2)
	viper.Set("market.book_depth_levels", 10)

	Convey("Given a running public websocket with a closed broadcast group", t, func() {
		pool := websocketTestPool(t)
		tree := websocketTestTree(t)

		endpoint, cleanup := testWebSocketEndpoint(t, func(conn *websocket.Conn) {
			time.Sleep(2 * time.Second)
			_ = conn.Close()
		})

		runCtx, cancelRun := context.WithCancel(t.Context())
		runnable := NewWebSocket(runCtx, pool, tree)
		runnable.symbols = []string{"BTC/USD"}

		So(pool.CreateBroadcastGroup("kraken:public").Close(), ShouldBeNil)

		go runnable.Run(endpoint)

		Reset(func() {
			cancelRun()
			cleanup()
		})

		So(waitUntil(2*time.Second, runnable.isConnected.Load), ShouldBeTrue)

		Convey("When subscribeMarket fails during Run", func() {
			Convey("It should keep subscribed false so subscribe retries", func() {
				So(waitUntil(2*time.Second, func() bool {
					return !runnable.subscribed.Load()
				}), ShouldBeTrue)
			})
		})
	})
}

func TestWebSocketConnect(t *testing.T) {
	viper.Set("system.network.connection.max_delay", 89)

	Convey("Given a public websocket", t, func() {
		pool := websocketTestPool(t)
		tree := websocketTestTree(t)
		ws := NewWebSocket(t.Context(), pool, tree)

		Convey("When Connect is called while already connected", func() {
			endpoint, cleanup := testWebSocketEndpoint(t, func(conn *websocket.Conn) {
				time.Sleep(2 * time.Second)
			})

			Reset(cleanup)

			So(ws.Connect(endpoint, 1), ShouldBeNil)

			Convey("It should return without redialing", func() {
				So(ws.Connect(endpoint, 1), ShouldBeNil)
				So(ws.isConnected.Load(), ShouldBeTrue)
			})
		})

		Convey("When Connect exceeds the max delay", func() {
			ws.connectMaxDelay = 2

			Convey("It should return an error immediately", func() {
				connectErr := ws.Connect(
					EndpointType("ws://127.0.0.1:1"),
					ws.connectMaxDelay+1,
				)

				So(connectErr, ShouldNotBeNil)
			})
		})

		Convey("When the server rejects the first dial then accepts", func() {
			var dialAttempts atomic.Int32

			upgrader := websocket.Upgrader{
				CheckOrigin: func(*http.Request) bool {
					return true
				},
			}

			server := httptest.NewServer(http.HandlerFunc(func(
				writer http.ResponseWriter,
				request *http.Request,
			) {
				if dialAttempts.Add(1) == 1 {
					writer.WriteHeader(http.StatusServiceUnavailable)
					return
				}

				conn, upgradeErr := upgrader.Upgrade(writer, request, nil)

				if upgradeErr != nil {
					return
				}

				time.Sleep(2 * time.Second)
				_ = conn.Close()
			}))

			endpoint := EndpointType(
				"ws" + strings.TrimPrefix(server.URL, "http"),
			)

			Reset(server.Close)

			Convey("It should retry and connect", func() {
				connectErr := ws.Connect(endpoint, 1)

				So(connectErr, ShouldBeNil)
				So(dialAttempts.Load(), ShouldBeGreaterThanOrEqualTo, 2)
				So(ws.isConnected.Load(), ShouldBeTrue)
			})
		})

		Convey("When ReadMessage fails on a running websocket", WithRunningWebSocket(t, pool, tree, func(conn *websocket.Conn) {
			time.Sleep(100 * time.Millisecond)
			_ = conn.Close()
			time.Sleep(2 * time.Second)
		}, func(runnable *WebSocket) {
			So(waitUntil(3*time.Second, func() bool {
				return runnable.Error() != nil
			}), ShouldBeTrue)
		}))
	})
}

func BenchmarkWebSocketConnect(b *testing.B) {
	viper.Set("system.network.connection.max_delay", 89)

	endpoint, cleanup := testWebSocketEndpoint(b, func(conn *websocket.Conn) {
		time.Sleep(5 * time.Second)
	})
	defer cleanup()

	pool := websocketTestPool(b)
	tree := websocketTestTree(b)

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		ws := NewWebSocket(b.Context(), pool, tree)

		if connectErr := ws.Connect(endpoint, 1); connectErr != nil {
			b.Fatal(connectErr)
		}

		_ = ws.Close()
	}
}
