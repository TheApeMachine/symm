package client

import (
	"context"
	"encoding/json"
	"net"
	"sync"
	"testing"

	"github.com/fasthttp/websocket"
	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/trader"
	"github.com/valyala/fasthttp"
)

type testWSServer struct {
	ln   net.Listener
	done chan struct{}
	url  string
}

func newTestWSServer(t *testing.T) *testWSServer {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")

	if err != nil {
		t.Fatal(err)
	}
	upgrader := websocket.FastHTTPUpgrader{}
	done := make(chan struct{})

	server := &fasthttp.Server{
		Handler: func(requestCtx *fasthttp.RequestCtx) {
			if !websocket.FastHTTPIsWebSocketUpgrade(requestCtx) {
				requestCtx.SetStatusCode(fasthttp.StatusBadRequest)
				return
			}

			if err := upgrader.Upgrade(requestCtx, func(conn *websocket.Conn) {
				defer conn.Close()

				for {
					_, payload, err := conn.ReadMessage()

					if err != nil {
						return
					}

					var frame map[string]any

					if json.Unmarshal(payload, &frame) != nil {
						continue
					}

					method, _ := frame["method"].(string)

					if method == "ping" {
						_ = conn.WriteJSON(map[string]string{"method": "pong"})
						continue
					}

					if method == "subscribe" {
						params, _ := frame["params"].(map[string]any)
						channel, _ := params["channel"].(string)

						_ = conn.WriteJSON(map[string]any{
							"channel": channel,
							"success": true,
						})
					}
				}
			}); err != nil {
				return
			}
		},
	}

	go func() {
		_ = server.Serve(listener)
	}()

	t.Cleanup(func() {
		close(done)
		_ = server.Shutdown()
		_ = listener.Close()
	})

	return &testWSServer{
		ln:   listener,
		done: done,
		url:  "ws://" + listener.Addr().String(),
	}
}

func (testServer *testWSServer) Close() {
	_ = testServer.ln.Close()
}

func TestPublicClientOpenInventorySymbols(t *testing.T) {
	convey.Convey("Given a wallet with open inventory", t, func() {
		publicClient := &PublicClient{}
		wallet := trader.NewWallet(trader.PaperWallet, "EUR", 200, 0.26)
		wallet.Inventory["SCOR"] = 1.5

		convey.Convey("It should derive Kraken symbols for open bases", func() {
			symbols := publicClient.openInventorySymbols(wallet)
			convey.So(symbols, convey.ShouldResemble, []string{"SCOR/EUR"})
		})
	})
}

func TestPublicClientConnect(t *testing.T) {
	convey.Convey("Given an in-memory websocket server", t, func() {
		testServer := newTestWSServer(t)
		defer testServer.Close()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
		defer pool.Close()

		publicClient := NewPublicClient(ctx, pool, testServer.url)

		convey.Convey("It should connect when the dial target is reachable", func() {
			convey.So(publicClient.Connect(), convey.ShouldBeNil)
			defer publicClient.Close()
		})
	})
}

func TestPublicClientReadLoopExitsOnDisconnect(t *testing.T) {
	convey.Convey("Given a websocket server that closes immediately", t, func() {
		listener, err := net.Listen("tcp", "127.0.0.1:0")

		if err != nil {
			t.Fatal(err)
		}

		upgrader := websocket.FastHTTPUpgrader{}
		server := &fasthttp.Server{
			Handler: func(requestCtx *fasthttp.RequestCtx) {
				if !websocket.FastHTTPIsWebSocketUpgrade(requestCtx) {
					requestCtx.SetStatusCode(fasthttp.StatusBadRequest)
					return
				}

				_ = upgrader.Upgrade(requestCtx, func(conn *websocket.Conn) {
					conn.Close()
				})
			},
		}

		go func() {
			_ = server.Serve(listener)
		}()

		t.Cleanup(func() {
			_ = server.Shutdown()
			_ = listener.Close()
		})

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
		defer pool.Close()

		publicClient := NewPublicClient(ctx, pool, "ws://"+listener.Addr().String())

		convey.Convey("It should not panic when the server closes the socket", func() {
			convey.So(publicClient.Connect(), convey.ShouldBeNil)
		})
	})
}

func BenchmarkPublicClientConnect(b *testing.B) {
	testServer := newTestWSServer(&testing.T{})
	defer testServer.Close()

	ctx := context.Background()
	pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
	defer pool.Close()

	var once sync.Once

	for b.Loop() {
		publicClient := NewPublicClient(ctx, pool, testServer.url)

		if err := publicClient.Connect(); err != nil {
			b.Fatalf("connect: %v", err)
		}

		once.Do(func() { _ = publicClient.Close() })
	}
}
