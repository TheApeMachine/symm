package response

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/trading"
)

func TestOnTradeQueueDepletion(t *testing.T) {
	configurePaperWallet()

	Convey("Given a resting post-only order with queue ahead", t, func() {
		broker.ResetQuoteCacheForTest()
		ctx := context.Background()
		pool := qpool.NewQ(ctx, 1, 4, nil)
		cache := broker.NewQuoteCache(ctx, pool)
		cache.InstallQuoteForTest(broker.Quote{
			Symbol: "BTC/EUR",
			Bid:    100,
			Ask:    100.5,
			Last:   100,
			Book: market.Book{
				Bids: []market.BookLevel{{Price: 100, Qty: 2.0}},
			},
		})

		orders := NewOrdersWithQuoteCache(
			ctx, pool, NewBalances(nil, nil, NewIdentifier()), NewIdentifier(), cache, nil, ZeroLatency(),
		)

		quote, quoteOK := cache.Snapshot("BTC/EUR")
		So(quoteOK, ShouldBeTrue)
		So(len(quote.Book.Bids), ShouldEqual, 1)
		So(broker.BookLevelQty(quote.Book.Bids, 100, 0.01), ShouldEqual, 2.0)

		orders.Send(&qpool.QValue[any]{Value: map[string]any{
			"method": trading.MethodAddOrder,
			"params": trading.AddParams{
				OrderType:  trading.Limit,
				Side:       trading.Buy,
				Symbol:     "BTC/EUR",
				OrderQty:   0.01,
				LimitPrice: 100,
				PostOnly:   true,
				ClOrdID:    "maker-entry",
			},
		}})

		resting := orders.makers["BTC/EUR"]

		Convey("Partial depletion leaves the order resting", func() {
			So(resting, ShouldNotBeNil)
			So(resting.limitPrice, ShouldEqual, 100)
			So(resting.queue.QueueAhead, ShouldEqual, 2.0)

			orders.onTrade("BTC/EUR", market.TradeUpdate{Side: "sell", Price: 100, Qty: 1.0})

			So(orders.makers["BTC/EUR"], ShouldNotBeNil)
			So(orders.makers["BTC/EUR"].queue.QueueAhead, ShouldEqual, 1.0)
			So(orders.makers["BTC/EUR"].queue.Ready(), ShouldBeFalse)
		})
	})
}

func TestPostOnlyMakerRestsUntilTradeDepletesQueue(t *testing.T) {
	configurePaperWallet()

	Convey("Given a post-only buy resting behind L2 queue depth", t, func() {
		broker.ResetQuoteCacheForTest()
		ctx := context.Background()
		pool := qpool.NewQ(ctx, 1, 4, nil)
		cache := broker.NewQuoteCache(ctx, pool)
		cache.InstallQuoteForTest(broker.Quote{
			Symbol: "BTC/EUR",
			Bid:    100,
			Ask:    100.5,
			Last:   100,
			Book: market.Book{
				Bids: []market.BookLevel{{Price: 100, Qty: 2.0}},
			},
		})

		raw := pool.CreateBroadcastGroup("raw", 10*time.Millisecond)
		sub := raw.Subscribe("test:maker", 32)
		ids := NewIdentifier()
		balances := NewBalances(raw, nil, ids)
		executions := NewExecutions(raw, balances, ids)
		orders := NewOrdersWithQuoteCache(ctx, pool, balances, ids, cache, nil, ZeroLatency())
		orders.Observe(executions)

		orders.Send(&qpool.QValue[any]{Value: map[string]any{
			"method": trading.MethodAddOrder,
			"params": trading.AddParams{
				OrderType:  trading.Limit,
				Side:       trading.Buy,
				Symbol:     "BTC/EUR",
				OrderQty:   0.01,
				LimitPrice: 100,
				PostOnly:   true,
				ClOrdID:    "maker-entry",
			},
		}})

		Convey("It does not fill until sell aggression depletes queue ahead", func() {
			So(expectNoFill(sub), ShouldBeTrue)

			orders.onTrade("BTC/EUR", market.TradeUpdate{Side: "sell", Price: 100, Qty: 1.0})

			So(expectNoFill(sub), ShouldBeTrue)

			orders.onTrade("BTC/EUR", market.TradeUpdate{Side: "sell", Price: 100, Qty: 2.0})

			exec := expectFill(sub)
			So(exec, ShouldNotBeNil)
			So(exec["side"], ShouldEqual, "buy")
			So(exec["qty"], ShouldEqual, 0.01)
		})
	})
}

func TestPostOnlyRejectsCrossingLimit(t *testing.T) {
	configurePaperWallet()

	Convey("Given a post-only buy that would cross the ask", t, func() {
		broker.ResetQuoteCacheForTest()
		ctx := context.Background()
		pool := qpool.NewQ(ctx, 1, 4, nil)
		cache := broker.NewQuoteCache(ctx, pool)
		cache.InstallQuoteForTest(broker.Quote{Symbol: "BTC/EUR", Bid: 99, Ask: 100, Last: 100})

		raw := pool.CreateBroadcastGroup("raw", 10*time.Millisecond)
		sub := raw.Subscribe("test:reject", 8)
		ids := NewIdentifier()
		orders := NewOrdersWithQuoteCache(ctx, pool, NewBalances(raw, nil, ids), ids, cache, nil, ZeroLatency())
		executions := NewExecutions(raw, nil, ids)
		orders.Observe(executions)

		orders.Send(&qpool.QValue[any]{Value: map[string]any{
			"method": trading.MethodAddOrder,
			"params": trading.AddParams{
				OrderType:  trading.Limit,
				Side:       trading.Buy,
				Symbol:     "BTC/EUR",
				OrderQty:   0.01,
				LimitPrice: 100,
				PostOnly:   true,
				ClOrdID:    "cross",
			},
		}})

		Convey("It rejects with zero quantity", func() {
			select {
			case frame := <-sub.Incoming:
				exec, _ := frame.Value.(map[string]any)
				So(exec["qty"], ShouldEqual, 0.0)
				So(exec["reason"], ShouldContainSubstring, "post-only")
			case <-time.After(200 * time.Millisecond):
				So("reject timeout", ShouldBeEmpty)
			}
		})
	})
}

func TestTakerFillUsesSlippageFill(t *testing.T) {
	configurePaperWallet()

	Convey("Given an L2 book and a market buy", t, func() {
		broker.ResetQuoteCacheForTest()
		ctx := context.Background()
		pool := qpool.NewQ(ctx, 1, 4, nil)
		cache := broker.NewQuoteCache(ctx, pool)
		cache.InstallQuoteForTest(broker.Quote{
			Symbol: "BTC/EUR",
			Bid:    100,
			Ask:    100,
			Last:   100,
			Book: market.Book{
				Asks: []market.BookLevel{
					{Price: 100, Qty: 1},
					{Price: 101, Qty: 1},
				},
			},
		})

		raw := pool.CreateBroadcastGroup("raw", 10*time.Millisecond)
		sub := raw.Subscribe("test:taker", 16)
		ids := NewIdentifier()
		balances := NewBalances(raw, nil, ids)
		executions := NewExecutions(raw, balances, ids)
		orders := NewOrdersWithQuoteCache(ctx, pool, balances, ids, cache, nil, ZeroLatency())
		orders.Observe(executions)

		orders.Send(&qpool.QValue[any]{Value: map[string]any{
			"method": trading.MethodAddOrder,
			"params": trading.AddParams{
				OrderType: trading.Market,
				Side:      trading.Buy,
				Symbol:    "BTC/EUR",
				OrderQty:  1.5,
				ClOrdID:   "market-buy",
			},
		}})

		Convey("It VWAPs through the ask book", func() {
			exec := expectFill(sub)
			So(exec, ShouldNotBeNil)
			So(exec["price"], ShouldAlmostEqual, 100.33333333333333, 1e-9)
		})
	})
}

func TestTakerFillDefersUntilLatencyElapses(t *testing.T) {
	configurePaperWallet()

	Convey("Given a positive one-way latency", t, func() {
		broker.ResetQuoteCacheForTest()
		ctx := context.Background()
		pool := qpool.NewQ(ctx, 1, 4, nil)
		cache := broker.NewQuoteCache(ctx, pool)
		seedQuote(cache, 100)

		raw := pool.CreateBroadcastGroup("raw", 10*time.Millisecond)
		sub := raw.Subscribe("test:latency", 8)
		ids := NewIdentifier()
		balances := NewBalances(raw, nil, ids)
		executions := NewExecutions(raw, balances, ids)
		orders := NewOrdersWithQuoteCache(ctx, pool, balances, ids, cache, nil, fixedLatency{50 * time.Millisecond})
		orders.Observe(executions)

		orders.Send(&qpool.QValue[any]{Value: map[string]any{
			"method": trading.MethodAddOrder,
			"params": trading.AddParams{
				OrderType: trading.Market,
				Side:      trading.Buy,
				Symbol:    "BTC/EUR",
				OrderQty:  0.01,
				ClOrdID:   "late",
			},
		}})

		Convey("It fills only after CheckPending observes the delay", func() {
			So(expectNoFill(sub), ShouldBeTrue)

			time.Sleep(60 * time.Millisecond)
			orders.CheckPending()

			So(expectFill(sub), ShouldNotBeNil)
		})
	})
}

type fixedLatency struct {
	delay time.Duration
}

func (sampler fixedLatency) Next() time.Duration { return sampler.delay }
