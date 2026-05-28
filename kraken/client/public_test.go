package client

import (
	"context"
	"encoding/json"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/fasthttp/websocket"
	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/ohlc"
	"github.com/theapemachine/symm/wallet"
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
		wallet := wallet.NewWallet(wallet.PaperWallet, "EUR", 200, 0.26)
		wallet.Inventory["SCOR"] = 1.5

		convey.Convey("It should derive Kraken symbols for open bases", func() {
			symbols := publicClient.openInventorySymbols(wallet)
			convey.So(symbols, convey.ShouldResemble, []string{"SCOR/EUR"})
		})
	})
}

func TestPublicClientSubscribeSymbolsQueuesWhenDisconnected(t *testing.T) {
	convey.Convey("Given a public client without a websocket connection", t, func() {
		publicClient := &PublicClient{
			subscribed:  make(map[string]struct{}),
			heldSymbols: make(map[string]struct{}),
		}

		convey.Convey("It should remember symbols for a later flush", func() {
			convey.So(publicClient.subscribeSymbols([]string{"BTC/EUR", "ETH/EUR"}), convey.ShouldBeNil)
			convey.So(publicClient.heldSymbols, convey.ShouldContainKey, "BTC/EUR")
			convey.So(publicClient.heldSymbols, convey.ShouldContainKey, "ETH/EUR")
		})
	})
}

func TestPublicClientReadTickerPublishesTickAndMark(t *testing.T) {
	convey.Convey("Given a Kraken ticker frame", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
		defer pool.Close()

		publicClient := NewPublicClient(ctx, pool, "ws://127.0.0.1:1")
		ticks := pool.CreateBroadcastGroup("tick", 10*time.Millisecond).
			Subscribe("test:public:ticker", 8)
		ui := pool.CreateBroadcastGroup("ui", 10*time.Millisecond).
			Subscribe("test:public:mark", 8)

		publicClient.read([]byte(`{
			"channel":"ticker",
			"type":"update",
			"data":[{"symbol":"BTC/EUR","last":50000,"bid":49999,"ask":50001,"timestamp":"2026-05-28T01:10:10.123456789Z"}]
		}`))

		convey.Convey("It should fan the source price to tick subscribers and UI marks", func() {
			select {
			case value := <-ticks.Incoming:
				row, ok := value.Value.(market.TickerRow)

				convey.So(ok, convey.ShouldBeTrue)
				convey.So(row.Symbol, convey.ShouldEqual, "BTC/EUR")
				convey.So(row.Last, convey.ShouldEqual, 50000)
			case <-time.After(time.Second):
				t.Fatal("timed out waiting for ticker row")
			}

			select {
			case value := <-ui.Incoming:
				payload, ok := value.Value.(map[string]any)

				convey.So(ok, convey.ShouldBeTrue)
				convey.So(payload["event"], convey.ShouldEqual, "mark")
				convey.So(payload["symbol"], convey.ShouldEqual, "BTC/EUR")
				convey.So(payload["price"], convey.ShouldEqual, 50000)
				convey.So(payload["ts"], convey.ShouldEqual, "2026-05-28T01:10:10.123456789Z")
			case <-time.After(time.Second):
				t.Fatal("timed out waiting for UI mark")
			}
		})
	})
}

func TestPublicClientReadOHLCPublishesTimedCandleAndMark(t *testing.T) {
	convey.Convey("Given a Kraken OHLC frame", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
		defer pool.Close()

		publicClient := NewPublicClient(ctx, pool, "ws://127.0.0.1:1")
		ohlcRows := pool.CreateBroadcastGroup("ohlc", 10*time.Millisecond).
			Subscribe("test:public:ohlc", 8)
		uiRows := pool.CreateBroadcastGroup("ui", 10*time.Millisecond).
			Subscribe("test:public:ohlc-ui", 8)
		expectedSec := time.Date(2026, 5, 28, 1, 10, 0, 0, time.UTC).Unix()

		publicClient.read([]byte(`{
			"channel":"ohlc",
			"type":"update",
			"data":[{
				"symbol":"H/EUR",
				"open":0.24,
				"high":0.246,
				"low":0.239,
				"close":0.245,
				"volume":1024,
				"interval":1,
				"interval_begin":"2026-05-28T01:10:00Z"
			}]
		}`))

		convey.Convey("It should preserve the exchange candle timestamp", func() {
			select {
			case value := <-ohlcRows.Incoming:
				row, ok := value.Value.(ohlc.Data)

				convey.So(ok, convey.ShouldBeTrue)
				convey.So(row.Symbol, convey.ShouldEqual, "H/EUR")
				convey.So(row.IntervalBegin.Unix(), convey.ShouldEqual, expectedSec)
			case <-time.After(time.Second):
				t.Fatal("timed out waiting for ohlc row")
			}

			select {
			case value := <-uiRows.Incoming:
				payload, ok := value.Value.(map[string]any)

				convey.So(ok, convey.ShouldBeTrue)
				convey.So(payload["event"], convey.ShouldEqual, "candle_bar")
				convey.So(payload["symbol"], convey.ShouldEqual, "H/EUR")
				convey.So(payload["ts"], convey.ShouldEqual, "2026-05-28T01:10:00Z")
				convey.So(payload["sec"], convey.ShouldEqual, expectedSec)
			case <-time.After(time.Second):
				t.Fatal("timed out waiting for candle bar")
			}

			select {
			case value := <-uiRows.Incoming:
				payload, ok := value.Value.(map[string]any)

				convey.So(ok, convey.ShouldBeTrue)
				convey.So(payload["event"], convey.ShouldEqual, "mark")
				convey.So(payload["symbol"], convey.ShouldEqual, "H/EUR")
				convey.So(payload["price"], convey.ShouldEqual, 0.245)
				convey.So(payload["ts"], convey.ShouldEqual, "2026-05-28T01:10:00Z")
			case <-time.After(time.Second):
				t.Fatal("timed out waiting for ohlc mark")
			}
		})
	})
}

func TestPublicClientTickKeepsListening(t *testing.T) {
	convey.Convey("Given wallet and subscription broadcasts", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
		defer pool.Close()

		publicClient := NewPublicClient(ctx, pool, "ws://127.0.0.1:1")
		walletGroup := pool.CreateBroadcastGroup("wallet", 0)
		subscriptionGroup := pool.CreateBroadcastGroup("subscriptions", 0)

		go func() {
			for ctx.Err() == nil {
				if err := publicClient.Tick(); err != nil {
					return
				}
			}
		}()

		walletGroup.Send(&qpool.QValue[any]{Value: wallet.NewWallet(wallet.PaperWallet, "EUR", 200, 0.26)})
		subscriptionGroup.Send(&qpool.QValue[any]{Value: []string{"BTC/EUR"}})
		subscriptionGroup.Send(&qpool.QValue[any]{Value: []string{"ETH/EUR"}})

		time.Sleep(10 * time.Millisecond)

		convey.Convey("It should accumulate held symbols across tick cycles", func() {
			publicClient.mu.Lock()
			_, hasBTC := publicClient.heldSymbols["BTC/EUR"]
			_, hasETH := publicClient.heldSymbols["ETH/EUR"]
			publicClient.mu.Unlock()

			convey.So(hasBTC, convey.ShouldBeTrue)
			convey.So(hasETH, convey.ShouldBeTrue)
		})

		cancel()
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
