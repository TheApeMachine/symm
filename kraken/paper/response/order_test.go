package response

import (
	"context"
	"testing"
	"time"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/bus"
	"github.com/theapemachine/symm/kraken/trading"
)

func paperOrders(ctx context.Context, pool *qpool.Q[any], cache *broker.QuoteCache) (*Orders, *qpool.BroadcastConsumer) {
	configurePaperWallet()

	if cache == nil {
		broker.ResetQuoteCacheForTest()
		cache = broker.NewQuoteCache(ctx, pool)
	}

	raw := bus.Group(pool, "raw", 10*time.Millisecond)
	ids := NewIdentifier()
	balances := NewBalances(raw, nil, ids)
	executions := NewExecutions(raw, balances, ids)
	orders := NewOrdersWithQuoteCache(ctx, pool, balances, ids, cache, nil, ZeroLatency())
	orders.Observe(executions)

	return orders, raw.Subscribe("test:exec", 32)
}

func armOrder(orders *Orders, params trading.AddParams) {
	orders.Send(&qpool.QValue[any]{Value: map[string]any{
		"method": trading.MethodAddOrder,
		"params": params,
	}})
}

func seedQuote(cache *broker.QuoteCache, last float64) {
	cache.InstallQuoteForTest(broker.Quote{Symbol: "BTC/EUR", Bid: last, Ask: last, Last: last})
}

func expectNoFill(sub *qpool.BroadcastConsumer) bool {
	deadline := time.Now().Add(40 * time.Millisecond)

	for time.Now().Before(deadline) {
		frame := sub.Poll()

		if frame == nil {
			time.Sleep(1 * time.Millisecond)
			continue
		}

		exec, ok := frame.Value.(map[string]any)

		if !ok || exec["channel"] != "executions" {
			continue
		}

		qty, ok := exec["qty"].(float64)

		if ok && qty > 0 {
			return false
		}
	}

	return true
}

func expectFill(sub *qpool.BroadcastConsumer) map[string]any {
	deadline := time.Now().Add(300 * time.Millisecond)

	for time.Now().Before(deadline) {
		frame := sub.Poll()

		if frame == nil {
			time.Sleep(1 * time.Millisecond)
			continue
		}

		exec, ok := frame.Value.(map[string]any)

		if !ok || exec["channel"] != "executions" {
			continue
		}

		qty, ok := exec["qty"].(float64)

		if ok && qty > 0 {
			return exec
		}
	}

	return nil
}

func drainUntilFill(sub *qpool.BroadcastConsumer) {
	deadline := time.Now().Add(300 * time.Millisecond)

	for time.Now().Before(deadline) {
		frame := sub.Poll()

		if frame == nil {
			time.Sleep(1 * time.Millisecond)
			continue
		}

		exec, ok := frame.Value.(map[string]any)

		if !ok || exec["channel"] != "executions" {
			continue
		}

		qty, ok := exec["qty"].(float64)

		if ok && qty > 0 {
			return
		}
	}
}

func TestPaperStopRestsThenFills(t *testing.T) {
	convey.Convey("Given a paper order handler with a seeded quote", t, func() {
		broker.ResetQuoteCacheForTest()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		pool := qpool.NewQ[any](ctx, 1, 4, nil)
		cache := broker.NewQuoteCache(ctx, pool)
		seedQuote(cache, 100)

		orders, sub := paperOrders(ctx, pool, cache)

		armOrder(orders, trading.AddParams{
			OrderType: trading.Limit, Side: trading.Buy, Symbol: "BTC/EUR",
			OrderQty: 1, ClOrdID: "seed",
		})
		drainUntilFill(sub)

		armOrder(orders, trading.AddParams{
			OrderType: trading.StopLoss, Side: trading.Sell, Symbol: "BTC/EUR",
			OrderQty: 1, ClOrdID: "stop", Triggers: &trading.Triggers{Reference: "last", Price: 98},
		})

		convey.Convey("It rests above the stop, then fills the moment price breaches", func() {
			seedQuote(cache, 99)
			orders.CheckTriggers()
			convey.So(expectNoFill(sub), convey.ShouldBeTrue)

			seedQuote(cache, 97)
			orders.CheckTriggers()

			exec := expectFill(sub)
			convey.So(exec, convey.ShouldNotBeNil)
			convey.So(exec["channel"], convey.ShouldEqual, "executions")
			convey.So(exec["side"], convey.ShouldEqual, "sell")
			convey.So(exec["qty"], convey.ShouldEqual, 1.0)
		})
	})
}

func TestPaperTrailingStopTracksPeak(t *testing.T) {
	convey.Convey("Given an armed trailing stop", t, func() {
		broker.ResetQuoteCacheForTest()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		pool := qpool.NewQ[any](ctx, 1, 4, nil)
		cache := broker.NewQuoteCache(ctx, pool)
		seedQuote(cache, 100)

		orders, sub := paperOrders(ctx, pool, cache)

		armOrder(orders, trading.AddParams{
			OrderType: trading.Limit, Side: trading.Buy, Symbol: "BTC/EUR",
			OrderQty: 1, ClOrdID: "seed",
		})
		drainUntilFill(sub)

		armOrder(orders, trading.AddParams{
			OrderType: trading.TrailingStop, Side: trading.Sell, Symbol: "BTC/EUR",
			OrderQty: 1, ClOrdID: "trail", Triggers: &trading.Triggers{Reference: "last", PriceType: "pct", Price: -2.0},
		})

		convey.Convey("The trail ratchets up with the peak and only fires below it", func() {
			seedQuote(cache, 120)
			orders.CheckTriggers()
			convey.So(expectNoFill(sub), convey.ShouldBeTrue)

			seedQuote(cache, 118)
			orders.CheckTriggers()
			convey.So(expectNoFill(sub), convey.ShouldBeTrue)

			seedQuote(cache, 117)
			orders.CheckTriggers()

			exec := expectFill(sub)
			convey.So(exec, convey.ShouldNotBeNil)
			convey.So(exec["qty"], convey.ShouldEqual, 1.0)
		})
	})
}

func TestPaperAddOrderAck(t *testing.T) {
	configurePaperWallet()

	convey.Convey("Given a paper order handler", t, func() {
		broker.ResetQuoteCacheForTest()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		pool := qpool.NewQ[any](ctx, 1, 4, nil)
		cache := broker.NewQuoteCache(ctx, pool)
		seedQuote(cache, 100)

		orders, _ := paperOrders(ctx, pool, cache)

		convey.Convey("add_order ack returns Kraken-shaped order identifiers", func() {
			ack := orders.Send(&qpool.QValue[any]{Value: map[string]any{
				"method": trading.MethodAddOrder,
				"params": trading.AddParams{
					OrderType: trading.Limit,
					Side:      trading.Buy,
					Symbol:    "BTC/EUR",
					OrderQty:  0.01,
					ClOrdID:   "entry-1",
				},
			}})

			result, ok := ack["result"].(map[string]any)
			convey.So(ok, convey.ShouldBeTrue)
			convey.So(result["cl_ord_id"], convey.ShouldEqual, "entry-1")
			convey.So(result["order_id"], convey.ShouldNotBeEmpty)
		})
	})
}
