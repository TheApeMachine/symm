package tests_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/tests"
	bookfixture "github.com/theapemachine/symm/tests/fixtures/book"
	tickerfixture "github.com/theapemachine/symm/tests/fixtures/ticker"
	tradefixture "github.com/theapemachine/symm/tests/fixtures/trade"
)

func TestMarketFrames(t *testing.T) {
	Convey("Given ticker, trade, and book update fixtures", t, func() {
		market := tests.NewMarket().
			Prefix(bookfixture.NewFixture(bookfixture.SNAPSHOT, 1)).
			Feed(tickerfixture.NewFixture(tickerfixture.UPDATE, 3)).
			Feed(tradefixture.NewFixture(tradefixture.UPDATE, 3)).
			Feed(bookfixture.NewFixture(bookfixture.UPDATE, 3))

		Convey("When the market timeline is replayed", func() {
			order := make([]string, 0)

			for frame := range market.Frames() {
				order = append(order, frame.Channel)
			}

			Convey("Then snapshots should lead and updates should round-robin", func() {
				So(order[0], ShouldEqual, "book")
				So(order[1:], ShouldResemble, []string{
					"ticker", "trade", "book",
					"ticker", "trade", "book",
					"ticker", "trade", "book",
				})
			})
		})
	})
}

func TestMarketReplay(t *testing.T) {
	Convey("Given a pumped ticker stream inside a market", t, func() {
		market := tests.NewMarket().
			Feed(tickerfixture.NewFixture(tickerfixture.UPDATE, 8))

		seen := map[string]int{}
		handlers := tests.Handlers{
			"ticker": func(payload []byte) {
				seen["ticker"]++
			},
		}

		Convey("When a scenario is applied and replayed", func() {
			tests.Replay(handlers, tests.Spike(market.Frames(), 4, 1.25, 1))

			Convey("Then registered handlers should receive every frame", func() {
				So(seen["ticker"], ShouldEqual, 8)
			})
		})
	})
}

func BenchmarkMarketFrames(b *testing.B) {
	market := tests.NewMarket().
		Feed(tickerfixture.NewFixture(tickerfixture.UPDATE, 32)).
		Feed(tradefixture.NewFixture(tradefixture.UPDATE, 32)).
		Feed(bookfixture.NewFixture(bookfixture.UPDATE, 32))

	b.ReportAllocs()

	for b.Loop() {
		for range market.Frames() {
		}
	}
}
