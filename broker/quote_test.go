package broker

import (
	"context"
	"fmt"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/market"
)

func TestQuoteCacheSnapshot(t *testing.T) {
	Convey("Given a quote cache with a seeded quote", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		cache := NewQuoteCache(ctx, nil)
		cache.InstallQuoteForTest(Quote{
			Symbol:    "BTC/EUR",
			Bid:       99,
			Ask:       100,
			Last:      99.5,
			UpdatedAt: time.Now().UTC(),
		})

		Convey("It should return the installed snapshot", func() {
			quote, ok := cache.Snapshot("BTC/EUR")

			So(ok, ShouldBeTrue)
			So(quote.Bid, ShouldEqual, 99)
			So(quote.Ask, ShouldEqual, 100)
		})

		Convey("It should report complete quotes", func() {
			So(cache.HasCompleteQuote("BTC/EUR"), ShouldBeTrue)
			So(cache.HasCompleteQuote("ETH/EUR"), ShouldBeFalse)
		})
	})
}

func TestQuoteCacheUpdateBook(t *testing.T) {
	Convey("Given a quote cache", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		cache := NewQuoteCache(ctx, nil)

		Convey("It should derive bid and ask from book levels", func() {
			cache.updateBook(market.Book{
				Symbol: "BTC/EUR",
				Bids:   []market.BookLevel{{Price: 98, Qty: 1}},
				Asks:   []market.BookLevel{{Price: 102, Qty: 1}},
			})

			quote, ok := cache.Snapshot("BTC/EUR")

			So(ok, ShouldBeTrue)
			So(quote.Bid, ShouldEqual, 98)
			So(quote.Ask, ShouldEqual, 102)
			So(quote.Last, ShouldAlmostEqual, 100, 0.0001)
		})
	})
}

func TestQuoteCacheSubscribe(t *testing.T) {
	Convey("Given a subscribed listener", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		cache := NewQuoteCache(ctx, nil)
		notified := make(chan Quote, 1)

		cache.Subscribe(func(symbol string, quote Quote) {
			if symbol == "BTC/EUR" {
				notified <- quote
			}
		})

		cache.updateTicker(market.TickerUpdate{
			Symbol: "BTC/EUR",
			Bid:    50_000,
			Ask:    50_100,
			Last:   50_050,
		})

		Convey("It should notify listeners on ticker updates", func() {
			select {
			case quote := <-notified:
				So(quote.Bid, ShouldEqual, 50_000)
				So(quote.Ask, ShouldEqual, 50_100)
			case <-time.After(time.Second):
				So("listener timeout", ShouldBeEmpty)
			}
		})
	})
}

func BenchmarkQuoteCacheSnapshot(b *testing.B) {
	ctx := context.Background()
	cache := NewQuoteCache(ctx, nil)
	cache.InstallQuoteForTest(Quote{
		Symbol:    "BTC/EUR",
		Bid:       99,
		Ask:       100,
		UpdatedAt: time.Now().UTC(),
	})

	for b.Loop() {
		_, _ = cache.Snapshot("BTC/EUR")
	}
}

func TestEnsureQuoteCache(t *testing.T) {
	Convey("Given a pool", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		pool := qpool.NewQ(ctx, 1, 4, nil)

		defer func() {
			cancel()
			pool.Close()
		}()

		Convey("It should return a shared cache instance", func() {
			first := EnsureQuoteCache(ctx, pool)
			second := EnsureQuoteCache(ctx, pool)

			So(first, ShouldEqual, second)
		})
	})
}

func TestEnsureQuoteCacheRecyclesOnCancel(t *testing.T) {
	Convey("Given a canceled quote-cache context", t, func() {
		firstCtx, firstCancel := context.WithCancel(context.Background())
		firstPool := qpool.NewQ(firstCtx, 1, 4, nil)

		first := EnsureQuoteCache(firstCtx, firstPool)
		firstCancel()
		ResetQuoteCacheForTest()
		firstPool.Close()

		secondCtx, secondCancel := context.WithCancel(context.Background())
		defer secondCancel()

		secondPool := qpool.NewQ(secondCtx, 1, 4, nil)
		defer secondPool.Close()

		second := EnsureQuoteCache(secondCtx, secondPool)

		Convey("It should construct a new cache instance", func() {
			So(fmt.Sprintf("%p", second), ShouldNotEqual, fmt.Sprintf("%p", first))
		})
	})
}
