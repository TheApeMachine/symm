package broker

import (
	"context"
	"testing"
	"time"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken/market"
)

func TestProjectExitBalance(t *testing.T) {
	convey.Convey("Given cash and one held lot", t, func() {
		cache := EnsureQuoteCache(context.Background(), nil)
		cache.InstallQuoteForTest(Quote{
			Symbol: "ETC/EUR",
			Bid:    11.87,
			Ask:    11.89,
			Last:   11.88,
			Book: market.Book{
				Bids: []market.BookLevel{{Price: 11.87, Qty: 1}},
			},
			UpdatedAt: time.Now().UTC(),
		})

		qty := 0.0993
		cash := 24.01
		takerFeePct := 0.26

		convey.Convey("It should add net sell proceeds after slippage and taker fee", func() {
			exitBalance, err := ProjectExitBalance(
				cash,
				map[string]float64{"ETC/EUR": qty},
				cache,
				takerFeePct,
			)

			convey.So(err, convey.ShouldBeNil)

			proceeds := qty * 11.87
			fee := proceeds * takerFeePct / 100

			convey.So(exitBalance, convey.ShouldAlmostEqual, cash+proceeds-fee, 1e-9)
		})

		convey.Convey("It should return an error when a held lot has no quote", func() {
			_, err := ProjectExitBalance(
				cash,
				map[string]float64{"LTC/EUR": 0.5},
				cache,
				takerFeePct,
			)

			convey.So(err, convey.ShouldNotBeNil)
		})
	})
}

func BenchmarkProjectExitBalance(b *testing.B) {
	cache := EnsureQuoteCache(context.Background(), nil)
	cache.InstallQuoteForTest(Quote{
		Symbol: "BTC/EUR",
		Bid:    99,
		Ask:    101,
		Last:   100,
		Book: market.Book{
			Bids: []market.BookLevel{
				{Price: 99, Qty: 2},
				{Price: 98.5, Qty: 3},
			},
		},
		UpdatedAt: time.Now().UTC(),
	})

	inventory := map[string]float64{"BTC/EUR": 0.01}

	for b.Loop() {
		_, _ = ProjectExitBalance(24, inventory, cache, 0.26)
	}
}
