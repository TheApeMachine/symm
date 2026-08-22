package websocket

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	gorillawebsocket "github.com/gorilla/websocket"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/types"
)

var upgrader = gorillawebsocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func TestFuturesLiveDispatch(t *testing.T) {
	Convey("Given a FuturesLive transport and a Thesis context", t, func() {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()

		thesis := types.NewThesis(ctx, nil)
		futures := NewFutures(ctx, thesis, "")

		Convey("Dispatching a ticker frame should populate futures ticker queue", func() {
			rawTicker := []byte(`{
				"feed": "ticker",
				"product_id": "PF_XBTUSD",
				"bid": 65000.5,
				"ask": 65001.0,
				"bid_size": 12.5,
				"ask_size": 8.0,
				"last": 65000.8,
				"open_interest": 42000000,
				"index": 64995.0,
				"funding_rate": 0.0001,
				"funding_rate_prediction": 0.00012,
				"time": 1700000000000
			}`)

			futures.DispatchFrame(rawTicker)
			symbol := thesis.Symbol("BTC/USD")
			So(symbol.HasPendingWork(types.SourceDerivatives), ShouldBeTrue)
		})

		Convey("Dispatching a trade frame should populate futures trade queue", func() {
			rawTrade := []byte(`{
				"feed": "trade",
				"product_id": "PF_ETHUSD",
				"side": "buy",
				"type": "fill",
				"price": 3500.25,
				"qty": 5.0,
				"uid": "trade-eth-1",
				"time": 1700000000100
			}`)

			futures.DispatchFrame(rawTrade)
			symbol := thesis.Symbol("ETH/USD")
			So(symbol.HasPendingWork(types.SourceDerivatives), ShouldBeTrue)
		})

		Convey("Dispatching a book delta frame should populate futures book queue", func() {
			rawBook := []byte(`{
				"feed": "book",
				"product_id": "PF_ZECUSD",
				"bids": [{"price": 45.5, "qty": 100.0}],
				"asks": [{"price": 45.6, "qty": 150.0}],
				"timestamp": 1700000000200
			}`)

			futures.DispatchFrame(rawBook)
			symbol := thesis.Symbol("ZEC/USD")
			So(symbol.HasPendingWork(types.SourceDerivatives), ShouldBeTrue)
		})

		Convey("Dispatching heartbeat or pong should be cleanly ignored", func() {
			futures.DispatchFrame([]byte(`{"event": "pong"}`))
			futures.DispatchFrame([]byte(`{"event": "heartbeat"}`))
			So(futures.Status(), ShouldEqual, types.INITIALIZING)
		})
	})
}

func TestFuturesLiveConnection(t *testing.T) {
	Convey("Given a mock Kraken Futures WebSocket server", t, func() {
		listener, err := net.Listen("tcp4", "127.0.0.1:0")

		if err != nil {
			return
		}

		server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			conn, err := upgrader.Upgrade(w, r, nil)

			if err != nil {
				return
			}

			defer conn.Close()

			for {
				var msg map[string]any
				err := conn.ReadJSON(&msg)

				if err != nil {
					return
				}

				if event, ok := msg["event"].(string); ok && event == "ping" {
					_ = conn.WriteJSON(map[string]string{"event": "pong"})
				}
			}
		}))
		server.Listener = listener
		server.Start()
		defer server.Close()

		wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

		ctx, cancel := context.WithTimeout(t.Context(), 500*time.Millisecond)
		defer cancel()

		thesis := types.NewThesis(ctx, nil)
		futures := NewFutures(ctx, thesis, wsURL)

		go func() {
			_ = futures.Run()
		}()

		time.Sleep(50 * time.Millisecond)

		Convey("It should connect and transition to READY status", func() {
			So(futures.Status(), ShouldEqual, types.READY)
			_ = futures.SubFuturesTicker([]string{"PF_XBTUSD"})
			_ = futures.SubFuturesTrades([]string{"PF_XBTUSD"})
			_ = futures.SubFuturesBook([]string{"PF_XBTUSD"})
		})
	})
}

func BenchmarkFuturesDispatchTicker(b *testing.B) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	thesis := types.NewThesis(ctx, nil)
	futures := NewFutures(ctx, thesis, "")

	rawTicker := []byte(`{
		"feed": "ticker",
		"product_id": "PF_XBTUSD",
		"bid": 65000.5,
		"ask": 65001.0,
		"bid_size": 12.5,
		"ask_size": 8.0,
		"last": 65000.8,
		"open_interest": 42000000,
		"index": 64995.0,
		"funding_rate": 0.0001,
		"funding_rate_prediction": 0.00012,
		"time": 1700000000000
	}`)

	for b.Loop() {
		futures.DispatchFrame(rawTicker)
	}
}

func BenchmarkFuturesDispatchTrade(b *testing.B) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	thesis := types.NewThesis(ctx, nil)
	futures := NewFutures(ctx, thesis, "")

	rawTrade := []byte(`{
		"feed": "trade",
		"product_id": "PF_XBTUSD",
		"side": "buy",
		"type": "fill",
		"price": 65000.8,
		"qty": 1.5,
		"uid": "benchmark-trade-1",
		"time": 1700000000000
	}`)

	for b.Loop() {
		futures.DispatchFrame(rawTrade)
	}
}
