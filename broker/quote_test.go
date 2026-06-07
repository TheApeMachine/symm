package broker

import (
	"context"
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

func TestQuoteCacheSnapshotFailsClosedOnForeignSymbol(t *testing.T) {
	Convey("Given an ETH/EUR slot corrupted with a BTC/EUR quote (a cross-symbol write)", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		cache := NewQuoteCache(ctx, nil)
		cache.slotFor("ETH/EUR").storeQuote(Quote{
			Symbol: "BTC/EUR", Bid: 52000, Ask: 52001, Last: 52000,
		})

		Convey("Snapshot fails closed instead of pricing ETH with the BTC quote", func() {
			_, ok := cache.Snapshot("ETH/EUR")
			So(ok, ShouldBeFalse)
		})

		Convey("A correctly-keyed quote is still returned", func() {
			cache.slotFor("XRP/EUR").storeQuote(Quote{
				Symbol: "XRP/EUR", Bid: 0.95, Ask: 0.96, Last: 0.955,
			})

			quote, ok := cache.Snapshot("XRP/EUR")

			So(ok, ShouldBeTrue)
			So(quote.Bid, ShouldEqual, 0.95)
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

func TestEnsureQuoteCacheConstructsInstance(t *testing.T) {
	Convey("Given a pool", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		pool := qpool.NewQ[any](ctx, 1, 4, nil)

		defer func() {
			cancel()
			pool.Close()
		}()

		Convey("EnsureQuoteCache should return a started cache", func() {
			cache := EnsureQuoteCache(ctx, pool)

			So(cache, ShouldNotBeNil)
			So(cache.started.Load(), ShouldBeTrue)
		})
	})
}
