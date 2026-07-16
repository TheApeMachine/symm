package types

import (
	"fmt"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/symm/kraken"

	. "github.com/smartystreets/goconvey/convey"
)

func liquidityTestRow(
	symbol string, bid, ask, bidQty, askQty, volume, vwap float64,
) kraken.TickerData {
	return kraken.TickerData{
		Symbol:    symbol,
		Bid:       decimal.NewFromFloat64(bid),
		BidQty:    bidQty,
		Ask:       decimal.NewFromFloat64(ask),
		AskQty:    askQty,
		Last:      decimal.NewFromFloat64((bid + ask) / 2),
		Volume:    volume,
		Vwap:      vwap,
		Timestamp: time.Now(),
	}
}

func TestQuoteNotional(t *testing.T) {
	Convey("Given a row with a reported vwap", t, func() {
		row := liquidityTestRow("BTC/USD", 99, 101, 1, 1, 10, 100)

		Convey("When QuoteNotional values it", func() {
			notional := QuoteNotional(row)

			Convey("Then it multiplies volume by vwap, not the mid price", func() {
				So(notional, ShouldEqual, 1000)
			})
		})
	})

	Convey("Given a row with no vwap reported yet", t, func() {
		row := liquidityTestRow("BTC/USD", 99, 101, 1, 1, 10, 0)

		Convey("When QuoteNotional values it", func() {
			notional := QuoteNotional(row)

			Convey("Then it falls back to the last trade price", func() {
				So(notional, ShouldEqual, 10*row.Last.Float64())
			})
		})
	})

	Convey("Given a row with no volume", t, func() {
		row := liquidityTestRow("BTC/USD", 99, 101, 1, 1, 0, 100)

		Convey("When QuoteNotional values it", func() {
			Convey("Then it is zero rather than dividing by an absent quantity", func() {
				So(QuoteNotional(row), ShouldEqual, 0)
			})
		})
	})
}

func TestExecutableDepth(t *testing.T) {
	Convey("Given a two-sided quote with asymmetric quantities", t, func() {
		row := liquidityTestRow("BTC/USD", 99, 101, 5, 2, 10, 100)

		Convey("When ExecutableDepth values it", func() {
			depth := ExecutableDepth(row)

			Convey("Then it uses the smaller side, valued at the mid price", func() {
				So(depth, ShouldEqual, 2*100)
			})
		})
	})

	Convey("Given a one-sided quote with no bid", t, func() {
		row := kraken.TickerData{
			Symbol: "BTC/USD",
			Bid:    decimal.NewFromFloat64(0),
			BidQty: 0,
			Ask:    decimal.NewFromFloat64(101),
			AskQty: 5,
		}

		Convey("When ExecutableDepth values it", func() {
			Convey("Then it is zero rather than pricing a one-sided book", func() {
				So(ExecutableDepth(row), ShouldEqual, 0)
			})
		})
	})
}

func TestCrossSectionMeasure(t *testing.T) {
	Convey("Given current ticker rows for three symbols", t, func() {
		crossSection := NewCrossSection()

		rows := []kraken.TickerData{
			liquidityTestRow("BTC/USD", 999, 1001, 2, 2, 1, 1000),
			liquidityTestRow("ETH/USD", 99, 101, 5, 5, 100, 100),
			liquidityTestRow("SOL/USD", 99, 101, 5, 5, 100, 100),
		}
		rows[0].ChangePct = 6
		rows[1].ChangePct = 2
		rows[2].ChangePct = -1
		crossSection.Measure(rows)

		Convey("Then every symbol contributes current liquidity and change metrics", func() {
			So(crossSection.Metrics, ShouldHaveLength, 3)

			for _, metric := range crossSection.Metrics {
				So(metric.QuoteNotional, ShouldBeGreaterThan, 0)
				So(metric.ExecutableDepth, ShouldBeGreaterThan, 0)
			}

			leader, threshold := crossSection.Leadership()
			So(leader, ShouldEqual, "BTC/USD")
			So(threshold, ShouldAlmostEqual, 0.02)
			So(crossSection.Breadth(), ShouldAlmostEqual, 2.0/3.0)
		})
	})
}

func TestCrossSectionMeasureReplacesSymbol(t *testing.T) {
	Convey("Given two current rows for the same symbol", t, func() {
		crossSection := NewCrossSection()
		first := liquidityTestRow("BTC/USD", 99, 101, 1, 1, 10, 100)
		second := liquidityTestRow("BTC/USD", 109, 111, 2, 2, 20, 110)
		second.Timestamp = first.Timestamp.Add(time.Second)
		second.ChangePct = 5

		crossSection.Measure([]kraken.TickerData{first, second})

		Convey("Then the tick retains only the latest calculated metric", func() {
			So(crossSection.Metrics, ShouldHaveLength, 1)
			So(crossSection.Metrics[0].LatestChange, ShouldAlmostEqual, 0.05)
			So(crossSection.Metrics[0].Volume, ShouldEqual, 20)
		})
	})
}

func TestCrossSectionMerge(t *testing.T) {
	Convey("Given independently measured cross-sections", t, func() {
		older := time.Unix(1, 0)
		newer := time.Unix(2, 0)
		crossSection := NewCrossSection()
		crossSection.Metrics = append(crossSection.Metrics, SymbolMetric{
			Symbol: "BTC/USD", At: older, LatestChange: 0.01,
		})
		crossSection.index["BTC/USD"] = 0
		incoming := NewCrossSection()
		incoming.Metrics = append(incoming.Metrics,
			SymbolMetric{Symbol: "BTC/USD", At: newer, LatestChange: 0.02},
			SymbolMetric{Symbol: "ETH/USD", At: newer, LatestChange: -0.01},
		)

		crossSection.Merge(incoming)

		Convey("It should retain one newest metric per symbol", func() {
			So(crossSection.Metrics, ShouldHaveLength, 2)
			So(crossSection.Metrics[crossSection.index["BTC/USD"]].At, ShouldEqual, newer)
			So(crossSection.Metrics[crossSection.index["BTC/USD"]].LatestChange, ShouldEqual, 0.02)
			So(crossSection.Metrics[crossSection.index["ETH/USD"]].LatestChange, ShouldEqual, -0.01)
		})
	})
}

/*
BenchmarkCrossSectionMeasure calculates the current 200-symbol tick including
liquidity, breadth, and leadership.
*/
func BenchmarkCrossSectionMeasure(b *testing.B) {
	rows := make([]kraken.TickerData, 200)

	for index := range rows {
		price := 100 + float64(index)
		rows[index] = liquidityTestRow(
			fmt.Sprintf("SYM%d/USD", index),
			price-1,
			price+1,
			5,
			5,
			10,
			price,
		)
		rows[index].ChangePct = float64(index%7) - 3
	}

	b.ReportAllocs()

	for b.Loop() {
		crossSection := NewCrossSection()
		crossSection.Measure(rows)
		crossSection.Breadth()
		crossSection.Leadership()
	}
}
