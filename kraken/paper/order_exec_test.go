package paper

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/public"
	"github.com/theapemachine/symm/kraken/trading"
)

func TestOrdersFillParams(t *testing.T) {
	Convey("Given a market order with a live quote", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		pool := qpool.NewQ(ctx, 1, 4, nil)
		ws := NewWebSocket(ctx, pool)
		defer ws.Close()

		broker.EnsureQuoteCache(ctx, pool).InstallQuoteForTest(broker.Quote{
			Symbol: "BTC/EUR",
			Bid:    99,
			Ask:    100,
			Last:   99.5,
		})

		orders := ws.sockets[public.OrdersChannel].(*Orders)
		message := orders.fillParams(trading.AddParams{
			OrderType: trading.Market,
			Side:      trading.Buy,
			Symbol:    "BTC/EUR",
			OrderQty:  0.01,
			ClOrdID:   "market-fill",
		})

		Convey("It should emit a filled execution", func() {
			So(message.Channel, ShouldEqual, public.ExecutionsChannel)
			So(string(message.Data), ShouldContainSubstring, `"cl_ord_id":"market-fill"`)
			So(string(message.Data), ShouldContainSubstring, `"order_status":"filled"`)
		})
	})
}

func TestOrdersDeferredMarketFill(t *testing.T) {
	Convey("Given measured one-way latency", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		latency := public.SharedNetworkLatency()
		latency.Reset()

		t.Cleanup(func() {
			latency.Reset()
		})

		for range 4 {
			latency.RecordRTT(100 * time.Millisecond)
		}

		pool := qpool.NewQ(ctx, 1, 4, nil)
		ws := NewWebSocket(ctx, pool)
		defer ws.Close()

		quotes := broker.EnsureQuoteCache(ctx, pool)
		quotes.InstallQuoteForTest(broker.Quote{
			Symbol: "BTC/EUR",
			Bid:    99,
			Ask:    100,
			Last:   99.5,
		})

		subscriber := ws.broadcasts["raw"].Subscribe("test:deferred-market-fill", 4)
		orders := ws.sockets[public.OrdersChannel].(*Orders)

		message := orders.fillParams(trading.AddParams{
			OrderType: trading.Market,
			Side:      trading.Buy,
			Symbol:    "BTC/EUR",
			OrderQty:  0.01,
			ClOrdID:   "deferred-market-fill",
		})

		Convey("It should defer the fill until the exchange-side quote arrives", func() {
			So(message.Channel, ShouldBeEmpty)

			go func() {
				time.Sleep(30 * time.Millisecond)
				quotes.InstallQuoteForTest(broker.Quote{
					Symbol: "BTC/EUR",
					Bid:    199,
					Ask:    200,
					Last:   199.5,
					Book: market.Book{
						Asks: []market.BookLevel{{Price: 200, Qty: 10}},
					},
				})
			}()

			var payload public.SocketMessage

			select {
			case frame := <-subscriber.Incoming:
				payload, _ = frame.Value.(public.SocketMessage)
			case <-time.After(300 * time.Millisecond):
				t.Fatal("timed out waiting for deferred market fill")
			}

			So(string(payload.Data), ShouldContainSubstring, `"cl_ord_id":"deferred-market-fill"`)
			So(string(payload.Data), ShouldContainSubstring, `"last_price":200`)
		})
	})
}

func BenchmarkOrdersFillParams(b *testing.B) {
	ctx := context.Background()
	pool := qpool.NewQ(ctx, 1, 4, nil)
	ws := NewWebSocket(ctx, pool)
	broker.EnsureQuoteCache(ctx, pool).InstallQuoteForTest(broker.Quote{
		Symbol: "BTC/EUR",
		Bid:    99,
		Ask:    100,
		Last:   99.5,
	})
	orders := ws.sockets[public.OrdersChannel].(*Orders)
	params := trading.AddParams{
		OrderType: trading.Market,
		Side:      trading.Buy,
		Symbol:    "BTC/EUR",
		OrderQty:  0.01,
	}

	for b.Loop() {
		_ = orders.fillParams(params)
	}
}
