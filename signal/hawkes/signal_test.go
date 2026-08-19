package hawkes

import (
	"context"
	"slices"
	"sync"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

func TestMeasure(t *testing.T) {
	Convey("Given Hawkes trade observations on one symbol", t, func() {
		signal := NewSignal(context.Background(), nil)
		market := types.NewSymbol("BTC/USD", nil)
		start := time.Unix(1_700_005_000, 0).UTC()

		for index, side := range []string{"buy", "sell", "buy"} {
			market.AppendTrade(kraken.TradeData{
				Symbol:    "BTC/USD",
				TradeID:   int64(index + 1),
				Side:      side,
				Timestamp: start.Add(time.Duration(index) * time.Second),
			})
		}

		err := signal.Measure(market)
		So(err, ShouldBeNil)
		measurements := slices.Collect(market.MarketMeasurements("category"))

		Convey("It should emit excitation metrics from nomagique", func() {
			So(len(measurements), ShouldBeGreaterThan, 0)
			latest := measurements[len(measurements)-1]
			So(signal.process.Symbols(), ShouldResemble, []string{"BTC/USD"})
			So(latest.Source, ShouldEqual, types.SourceHawkes)
			So(latest.Symbol, ShouldEqual, "BTC/USD")
			So(latest.Maturity, ShouldBeGreaterThanOrEqualTo, 0)
			So(latest.Sample(types.MetricEventCount, types.SideNone).Raw,
				ShouldBeGreaterThan, 0)
			So(latest.Sample(types.MetricEventCount, types.SideBuy).Raw,
				ShouldBeGreaterThanOrEqualTo, 0)
			So(latest.Sample(types.MetricEventCount, types.SideSell).Raw,
				ShouldBeGreaterThanOrEqualTo, 0)
			So(latest.Sample(types.MetricHypothesisSeparation, types.SideNone).Unit,
				ShouldEqual, types.UnitDimensionless)
		})
	})

	Convey("Given a single first trade", t, func() {
		signal := NewSignal(context.Background(), nil)
		market := types.NewSymbol("BTC/USD", nil)
		at := time.Unix(1_700_007_000, 0).UTC()
		market.AppendTrade(kraken.TradeData{
			Symbol:    "BTC/USD",
			TradeID:   1,
			Side:      "buy",
			Timestamp: at,
		})

		err := signal.Measure(market)
		So(err, ShouldBeNil)
		measurements := slices.Collect(market.MarketMeasurements("category"))

		Convey("It should emit an immature measurement with unready separation", func() {
			So(measurements, ShouldHaveLength, 1)
			measurement := measurements[0]
			So(measurement.Source, ShouldEqual, types.SourceHawkes)
			So(measurement.Symbol, ShouldEqual, "BTC/USD")
			So(measurement.At, ShouldResemble, at)
			So(measurement.Maturity, ShouldEqual, 0)
			So(measurement.Sample(types.MetricEventCount, types.SideNone).Raw,
				ShouldEqual, 1)
			So(measurement.Sample(types.MetricEventCount, types.SideBuy).Raw,
				ShouldEqual, 1)
			So(measurement.Sample(types.MetricHypothesisSeparation, types.SideNone).Raw,
				ShouldEqual, 0)
			So(measurement.Sample(types.MetricHypothesisSeparation, types.SideNone).Normalized,
				ShouldBeNil)
		})

		Convey("When asked again with no new trades", func() {
			err := signal.Measure(market)
			So(err, ShouldBeNil)
			again := slices.Collect(market.MarketMeasurements("category"))

			Convey("It should republish the last measurement", func() {
				So(again, ShouldHaveLength, 1)
				So(again[0].Symbol, ShouldEqual, "BTC/USD")
				So(again[0].Maturity, ShouldEqual, 0)
				So(again[0].Sample(types.MetricEventCount, types.SideNone).Raw,
					ShouldEqual, 1)
				So(again[0].Sample(types.MetricHypothesisSeparation, types.SideNone).Normalized,
					ShouldBeNil)
			})
		})
	})

	Convey("Given a market that has never traded", t, func() {
		signal := NewSignal(context.Background(), nil)
		market := types.NewSymbol("BTC/USD", nil)

		Convey("It should not invent a measurement", func() {
			err := signal.Measure(market)
			So(err, ShouldBeNil)
			So(slices.Collect(market.MarketMeasurements("category")), ShouldBeEmpty)
		})
	})

	Convey("Given trades for two independent symbols", t, func() {
		signal := NewSignal(context.Background(), nil)
		market := types.NewSymbol("AAA/USD", nil)
		start := time.Unix(1_700_006_000, 0).UTC()
		market.AppendTrade(kraken.TradeData{Symbol: "AAA/USD", Side: "buy", Timestamp: start})
		market.AppendTrade(kraken.TradeData{Symbol: "BBB/USD", Side: "sell", Timestamp: start})

		err := signal.Measure(market)
		So(err, ShouldBeNil)
		measurements := slices.Collect(market.MarketMeasurements("category"))

		Convey("It should keep each symbol's estimator state apart", func() {
			So(measurements, ShouldHaveLength, 2)
			So(signal.process.Symbols(), ShouldResemble,
				[]string{"AAA/USD", "BBB/USD"})
		})
	})
}

func TestSeenTrade(t *testing.T) {
	Convey("Given Hawkes arrivals sharing a timestamp", t, func() {
		signal := &Signal{lastTrade: &sync.Map{}}
		at := time.Unix(1_700_006_500, 0).UTC()
		first := kraken.TradeData{Symbol: "BTC/USD", TradeID: 1, Timestamp: at}
		second := kraken.TradeData{Symbol: "BTC/USD", TradeID: 2, Timestamp: at}
		regressed := kraken.TradeData{
			Symbol: "BTC/USD", TradeID: 3, Timestamp: at.Add(-time.Nanosecond),
		}

		Convey("It should admit each identity once and reject replay or temporal regression", func() {
			So(signal.seenTrade(first), ShouldBeFalse)
			signal.commitTrade(first)
			So(signal.seenTrade(first), ShouldBeTrue)
			So(signal.seenTrade(second), ShouldBeFalse)
			signal.commitTrade(second)
			So(signal.seenTrade(second), ShouldBeTrue)
			So(signal.seenTrade(regressed), ShouldBeTrue)
		})
	})
}

func BenchmarkMeasure(b *testing.B) {
	signal := NewSignal(context.Background(), nil)
	market := types.NewSymbol("BTC/USD", nil)
	start := time.Unix(1_700_005_000, 0).UTC()
	iteration := 0

	b.ReportAllocs()

	for b.Loop() {
		side := "buy"

		if iteration%2 != 0 {
			side = "sell"
		}

		market.AppendTrade(kraken.TradeData{
			Symbol:    "BTC/USD",
			TradeID:   int64(iteration + 1),
			Side:      side,
			Timestamp: start.Add(time.Duration(iteration) * time.Second),
		})
		err := signal.Measure(market)
		So(err, ShouldBeNil)
		iteration++
	}
}
