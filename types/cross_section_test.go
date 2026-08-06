package types

import (
	"fmt"
	"testing"
	"time"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/api-go/v2/pkg/decimal"
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

func TestCrossSectionQuoteNotional(t *testing.T) {
	Convey("Given a row with a reported vwap", t, func() {
		crossSection := NewCrossSection()
		row := liquidityTestRow("BTC/USD", 99, 101, 1, 1, 10, 100)

		Convey("When QuoteNotional values it", func() {
			notional := crossSection.QuoteNotional(row)

			Convey("Then it multiplies volume by vwap, not the mid price", func() {
				So(notional, ShouldEqual, 1000)
			})
		})
	})

	Convey("Given a row with no vwap reported yet", t, func() {
		crossSection := NewCrossSection()
		row := liquidityTestRow("BTC/USD", 99, 101, 1, 1, 10, 0)

		Convey("When QuoteNotional values it", func() {
			notional := crossSection.QuoteNotional(row)

			Convey("Then it falls back to the last trade price", func() {
				So(notional, ShouldEqual, 10*row.Last.Float64())
			})
		})
	})
}

func TestCrossSectionExecutableDepth(t *testing.T) {
	Convey("Given a two-sided quote with asymmetric quantities", t, func() {
		crossSection := NewCrossSection()
		row := liquidityTestRow("BTC/USD", 99, 101, 5, 2, 10, 100)

		Convey("When ExecutableDepth values it", func() {
			depth := crossSection.ExecutableDepth(row)

			Convey("Then it uses the smaller side, valued at the mid price", func() {
				So(depth, ShouldEqual, 2*100)
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
			count := 0

			crossSection.Metrics.Range(func(_, _ any) bool {
				count++
				return true
			})

			So(count, ShouldEqual, 3)

			leader, threshold := crossSection.Leadership()
			So(leader, ShouldEqual, "BTC/USD")
			So(threshold, ShouldAlmostEqual, 0.02)
			So(crossSection.Breadth(), ShouldAlmostEqual, 2.0/3.0)

			value, ok := crossSection.Metrics.Load("BTC/USD")
			So(ok, ShouldBeTrue)
			So(value.(SymbolMetric).RelativeSpread, ShouldAlmostEqual, 0.002)
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
			value, ok := crossSection.Metrics.Load("BTC/USD")
			So(ok, ShouldBeTrue)

			metric := value.(SymbolMetric)
			So(metric.LatestChange, ShouldAlmostEqual, 0.05)
			So(metric.Volume, ShouldEqual, 20)
		})
	})
}

/*
TestCrossSectionLeadership proves a moving symbol remains identifiable when
the peer median is exactly zero rather than being erased with the scale.
*/
func TestCrossSectionLeadership(t *testing.T) {
	Convey("Given one moving symbol and two unchanged peers", t, func() {
		crossSection := NewCrossSection()
		crossSection.Metrics.Store("BTC/USD", SymbolMetric{Symbol: "BTC/USD", LatestChange: 0.05})
		crossSection.Metrics.Store("ETH/USD", SymbolMetric{Symbol: "ETH/USD"})
		crossSection.Metrics.Store("SOL/USD", SymbolMetric{Symbol: "SOL/USD"})

		Convey("When leadership is measured", func() {
			leader, threshold := crossSection.Leadership()

			Convey("Then the zero median should not hide the actual leader", func() {
				So(leader, ShouldEqual, "BTC/USD")
				So(threshold, ShouldEqual, 0)
			})
		})
	})
}

func TestCrossSectionBreadthEmpty(t *testing.T) {
	Convey("Given an empty cross section", t, func() {
		crossSection := NewCrossSection()

		Convey("When Breadth is calculated", func() {
			Convey("Then it is zero rather than NaN", func() {
				So(crossSection.Breadth(), ShouldEqual, 0)
			})
		})
	})
}

func TestCrossSectionMarshalJSON(t *testing.T) {
	Convey("Given measured peer metrics", t, func() {
		crossSection := NewCrossSection()
		row := liquidityTestRow("BTC/USD", 99, 101, 1, 1, 10, 100)
		row.ChangePct = 4
		crossSection.Measure([]kraken.TickerData{row})

		Convey("When the cross section is marshaled and restored", func() {
			payload, err := sonic.Marshal(crossSection)
			So(err, ShouldBeNil)

			restored := NewCrossSection()
			So(sonic.Unmarshal(payload, restored), ShouldBeNil)

			value, ok := restored.Metrics.Load("BTC/USD")
			So(ok, ShouldBeTrue)
			So(value.(SymbolMetric).LatestChange, ShouldAlmostEqual, 0.04)
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
