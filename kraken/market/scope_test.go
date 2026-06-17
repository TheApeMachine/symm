package market

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestPayloadSymbols(testingTB *testing.T) {
	Convey("Given feed artifacts", testingTB, func() {
		Convey("When PayloadSymbols reads a ticker artifact", func() {
			artifact := TickerFeedArtifact(TickerUpdates{
				{Symbol: "BTC/USD", Last: 100},
				{Symbol: "ETH/USD", Last: 50},
			})

			symbols := PayloadSymbols(artifact)

			Convey("It should return unique symbols", func() {
				So(symbols, ShouldResemble, []string{"BTC/USD", "ETH/USD"})
			})
		})

		Convey("When VisitTickers walks a ticker artifact", func() {
			artifact := TickerFeedArtifact(TickerUpdates{
				{Symbol: "BTC/USD", Last: 100},
				{Symbol: "ETH/USD", Last: 0},
			})

			visited := make([]string, 0, 1)

			VisitTickers(artifact, func(symbol string, last float64) bool {
				visited = append(visited, symbol)
				So(last, ShouldEqual, 100)

				return true
			})

			Convey("It should skip rows without last price", func() {
				So(visited, ShouldResemble, []string{"BTC/USD"})
			})
		})
	})
}

func TestFeedArtifacts(testingTB *testing.T) {
	Convey("Given kraken updates", testingTB, func() {
		now := time.Date(2026, 6, 17, 12, 0, 0, 0, time.UTC)

		Convey("When BookFeedArtifact wraps a book update", func() {
			artifact := BookFeedArtifact(BookUpdates{
				{
					Symbol:    "BTC/USD",
					Timestamp: now,
					Bids:      []BookLevel{{Price: 99, Qty: 1}},
					Asks:      []BookLevel{{Price: 101, Qty: 1}},
				},
			})

			Convey("It should expose the book role", func() {
				So(PayloadSymbols(artifact), ShouldResemble, []string{"BTC/USD"})
			})
		})
	})
}

func BenchmarkPayloadSymbols(benchmark *testing.B) {
	artifact := TickerFeedArtifact(TickerUpdates{
		{Symbol: "BTC/USD", Last: 100},
	})

	for benchmark.Loop() {
		_ = PayloadSymbols(artifact)
	}
}
