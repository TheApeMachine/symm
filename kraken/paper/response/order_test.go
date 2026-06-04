package response

import (
	"context"
	"testing"
	"time"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken/trading"
)

func armOrder(orders *Orders, params trading.AddParams) {
	orders.Send(&qpool.QValue[any]{Value: map[string]any{
		"method": trading.MethodAddOrder,
		"params": params,
	}})
}

func seedQuote(cache *broker.QuoteCache, last float64) {
	cache.InstallQuoteForTest(broker.Quote{Symbol: "BTC/EUR", Bid: last, Ask: last, Last: last})
}

func expectNoFill(sub *qpool.Subscriber) bool {
	select {
	case <-sub.Incoming:
		return false
	case <-time.After(40 * time.Millisecond):
		return true
	}
}

func expectFill(sub *qpool.Subscriber) map[string]any {
	select {
	case frame := <-sub.Incoming:
		exec, _ := frame.Value.(map[string]any)
		return exec
	case <-time.After(300 * time.Millisecond):
		return nil
	}
}

func TestPaperStopRestsThenFills(t *testing.T) {
	convey.Convey("Given a paper order handler with a seeded quote", t, func() {
		broker.ResetQuoteCacheForTest()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		pool := qpool.NewQ(ctx, 1, 4, nil)
		cache := broker.EnsureQuoteCache(ctx, pool)
		seedQuote(cache, 100)

		orders := NewOrders(ctx, pool, nil)
		sub := pool.CreateBroadcastGroup("raw", 10*time.Millisecond).Subscribe("test:exec", 8)

		armOrder(orders, trading.AddParams{
			OrderType: trading.StopLoss, Side: trading.Sell, Symbol: "BTC/EUR",
			OrderQty: 1, ClOrdID: "stop", Triggers: &trading.Triggers{Reference: "last", Price: 98},
		})

		convey.Convey("It rests above the stop, then fills the moment price breaches", func() {
			seedQuote(cache, 99)
			orders.CheckTriggers()
			convey.So(expectNoFill(sub), convey.ShouldBeTrue) // armed, not fired

			seedQuote(cache, 97)
			orders.CheckTriggers()

			exec := expectFill(sub)
			convey.So(exec, convey.ShouldNotBeNil)
			convey.So(exec["channel"], convey.ShouldEqual, "executions")
			convey.So(exec["side"], convey.ShouldEqual, "sell")
			convey.So(exec["reason"], convey.ShouldEqual, "trigger")
			convey.So(exec["qty"], convey.ShouldEqual, 1.0)
		})
	})
}

func TestPaperTrailingStopTracksPeak(t *testing.T) {
	convey.Convey("Given an armed trailing stop", t, func() {
		broker.ResetQuoteCacheForTest()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		pool := qpool.NewQ(ctx, 1, 4, nil)
		cache := broker.EnsureQuoteCache(ctx, pool)
		seedQuote(cache, 100) // peak starts here when armed

		orders := NewOrders(ctx, pool, nil)
		sub := pool.CreateBroadcastGroup("raw", 10*time.Millisecond).Subscribe("test:trail", 8)

		// Desk encodes a 2% trail as a negative percent.
		armOrder(orders, trading.AddParams{
			OrderType: trading.TrailingStop, Side: trading.Sell, Symbol: "BTC/EUR",
			OrderQty: 1, ClOrdID: "trail", Triggers: &trading.Triggers{Reference: "last", PriceType: "pct", Price: -2.0},
		})

		convey.Convey("The trail ratchets up with the peak and only fires below it", func() {
			seedQuote(cache, 120) // peak -> 120, level 117.6
			orders.CheckTriggers()
			convey.So(expectNoFill(sub), convey.ShouldBeTrue)

			seedQuote(cache, 118) // above 117.6 -> still resting
			orders.CheckTriggers()
			convey.So(expectNoFill(sub), convey.ShouldBeTrue)

			seedQuote(cache, 117) // below the trailed level -> fills
			orders.CheckTriggers()

			exec := expectFill(sub)
			convey.So(exec, convey.ShouldNotBeNil)
			convey.So(exec["reason"], convey.ShouldEqual, "trigger")
		})
	})
}
