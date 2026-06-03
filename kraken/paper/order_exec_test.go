package paper

import (
	"context"
	"encoding/json"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/broker"
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
			channel, _ := message["channel"].(string)
			data, _ := message["data"].(json.RawMessage)

			So(channel, ShouldEqual, public.ExecutionsChannel)
			So(string(data), ShouldContainSubstring, `"cl_ord_id":"market-fill"`)
			So(string(data), ShouldContainSubstring, `"order_status":"filled"`)
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
