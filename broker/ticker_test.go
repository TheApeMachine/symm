package broker

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestTickerUpdate(testingTB *testing.T) {
	Convey("Given a ticker book with an initial full quote", testingTB, func() {
		ticker := NewTicker()

		So(ticker.Update(tickerFrame([]map[string]any{{
			"symbol": "BTC/USD",
			"bid":    99.0,
			"ask":    101.0,
			"last":   100.0,
		}})), ShouldBeNil)

		Convey("When a partial update carries only the bid", func() {
			err := ticker.Update(tickerFrame([]map[string]any{{
				"symbol": "BTC/USD",
				"bid":    100.0,
			}}))
			quote, ok := ticker.Quote("BTC/USD")

			Convey("Then it merges the partial row into the previous quote", func() {
				So(err, ShouldBeNil)
				So(ok, ShouldBeTrue)
				So(quote.Bid, ShouldEqual, 100)
				So(quote.Ask, ShouldEqual, 101)
				So(quote.Last, ShouldEqual, 100)
			})
		})
	})

	Convey("Given a ticker book without a prior quote", testingTB, func() {
		ticker := NewTicker()

		Convey("When a partial update carries no executable price", func() {
			err := ticker.Update(tickerFrame([]map[string]any{{
				"symbol": "ETH/USD",
				"volume": 42.0,
			}}))
			_, ok := ticker.Quote("ETH/USD")

			Convey("Then it ignores the row without failing the stream", func() {
				So(err, ShouldBeNil)
				So(ok, ShouldBeFalse)
			})
		})
	})
}

func tickerFrame(rows []map[string]any) map[string]any {
	return map[string]any{
		"channel": "ticker",
		"data":    rows,
	}
}

func BenchmarkTickerUpdate(benchmarkTB *testing.B) {
	ticker := NewTicker()
	frame := map[string]any{
		"channel": "ticker",
		"data": []any{
			map[string]any{
				"symbol": "BTC/USD",
				"bid":    99.0,
				"ask":    101.0,
				"last":   100.0,
			},
		},
	}

	benchmarkTB.ReportAllocs()
	for index := 0; index < benchmarkTB.N; index++ {
		if err := ticker.Update(frame); err != nil {
			benchmarkTB.Fatal(err)
		}
	}
}
