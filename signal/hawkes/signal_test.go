package hawkes

import (
	"context"
	"sync"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/algorithm/excitation"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

func TestMeasure(t *testing.T) {
	Convey("Given one new trade on each Thesis fanout", t, func() {
		ui := make(chan []byte, 1)
		signal := &Signal{
			ctx:     context.Background(),
			process: excitation.NewProcess(),
			sample:  excitation.NewSample(),
			ui:      ui,
		}
		start := time.Unix(1_700_005_000, 0).UTC()
		thesis := types.NewThesis(t.Context(), nil)
		var measurements []*types.Measurement
		var latest *types.Measurement
		expectedCounts := []float64{1, 1, 2}

		for index, side := range []string{"buy", "sell", "buy"} {
			thesis.AppendTrade(kraken.TradeData{
				Symbol:    "BTC/USD",
				TradeID:   int64(index + 1),
				Side:      side,
				Timestamp: start.Add(time.Duration(index) * time.Second),
			})
			measurements = signal.Measure(thesis)

			So(measurements, ShouldHaveLength, 1)
			So(measurements[0].Sample(
				types.MetricEventCount,
				types.SideNone,
			).Raw, ShouldEqual, expectedCounts[index])
			So(measurements[0].Sample(types.MetricSNR, types.SideNone).Normalized,
				ShouldBeNil)

			So(string(<-ui), ShouldContainSubstring, `"measurements"`)
			latest = measurements[0]

			thesis.AppendMeasurements(types.SourceHawkes, measurements, false)
			So(thesis.MarketTrades(types.SourceHawkes), ShouldBeEmpty)
		}

		Convey("It should leave retained arrivals and fit state in Nomagique", func() {
			So(signal.process.Symbols(), ShouldResemble, []string{"BTC/USD"})
			So(latest.Metrics, ShouldHaveLength, 23)
			So(latest.ObservedFrom, ShouldResemble, start)
			So(latest.At, ShouldResemble, start.Add(2*time.Second))
			So(latest.Horizon, ShouldEqual, 2*time.Second)
			So(latest.Sample(types.MetricEventCount, types.SideNone).Raw,
				ShouldEqual, 2.0)
			So(latest.Sample(types.MetricEventCount, types.SideBuy).Raw,
				ShouldEqual, 1.0)
			So(latest.Sample(types.MetricEventCount, types.SideSell).Raw,
				ShouldEqual, 1.0)
			So(latest.Sample(types.MetricArrivalRate, types.SideBuy).Raw,
				ShouldEqual, 0.5)
			So(latest.Sample(types.MetricArrivalRate, types.SideSell).Raw,
				ShouldEqual, 0.5)
		})

		Convey("It should not label unbounded expectations as normalized", func() {
			for _, metric := range []types.MetricType{
				types.MetricImmediateOffspring,
				types.MetricTotalDescendants,
			} {
				So(latest.Sample(metric, types.SideBuy).Normalized,
					ShouldBeNil)
				So(latest.Sample(metric, types.SideSell).Normalized,
					ShouldBeNil)
			}
		})
	})

	Convey("Given a liquid market and an unrelated thin market", t, func() {
		signal := &Signal{
			ctx:     context.Background(),
			process: excitation.NewProcess(),
			sample:  excitation.NewSample(),
		}
		start := time.Unix(1_700_006_000, 0).UTC()
		thesis := types.NewThesis(t.Context(), nil)
		liquid := make([]kraken.TradeData, 0, 80)

		for index := range 80 {
			side := "buy"

			if index%2 != 0 {
				side = "sell"
			}

			liquid = append(liquid, kraken.TradeData{
				Symbol:    "BTC/USD",
				Side:      side,
				Timestamp: start.Add(time.Duration(index) * time.Second),
			})
		}

		thesis.Trades.Store("BTC/USD", liquid)
		thesis.Trades.Store("THIN/USD", []kraken.TradeData{
			{Symbol: "THIN/USD", Side: "buy", Timestamp: start},
			{Symbol: "THIN/USD", Side: "sell", Timestamp: start.Add(time.Second)},
		})

		measurements := signal.Measure(thesis)

		Convey("It should not let the thin market veto Hawkes readiness", func() {
			So(len(measurements) > 0, ShouldBeTrue)
		})

		Convey("It should compare the fitted directional offspring totals", func() {
			var measurement *types.Measurement

			for _, candidate := range measurements {
				if candidate.Symbol == "BTC/USD" {
					measurement = candidate
					break
				}
			}

			So(measurement, ShouldNotBeNil)
			expectedSNR, snrReady := types.MeasurementSignalNoiseRatio(
				types.SourceHawkes,
				measurement.Metrics,
			)
			So(snrReady, ShouldBeTrue)
			So(measurement.Sample(types.MetricSNR, types.SideNone).Raw,
				ShouldEqual, expectedSNR)
		})
	})

	Convey("Given trades for symbols whose marks arrive out of order", t, func() {
		signal := &Signal{
			ctx:     context.Background(),
			process: excitation.NewProcess(),
			sample:  excitation.NewSample(),
		}
		start := time.Unix(1_700_006_000, 0).UTC()
		thesis := types.NewThesis(t.Context(), nil)

		// Ingestion receives the sell first and keeps the stored stream causal.
		// The sell lands after the last buy in exchange time, which is what makes
		// the horizon span both marks rather than the buy side alone.
		thesis.AppendTrade(kraken.TradeData{
			Symbol: "AAA/USD", Side: "sell", Timestamp: start.Add(2 * time.Second),
		})
		thesis.AppendTrade(kraken.TradeData{
			Symbol: "AAA/USD", Side: "buy", Timestamp: start,
		})
		thesis.AppendTrade(kraken.TradeData{
			Symbol: "AAA/USD", Side: "buy", Timestamp: start.Add(time.Second),
		})

		measurements := signal.Measure(thesis)

		Convey("It should emit one row for the symbol", func() {
			So(measurements, ShouldHaveLength, 1)
			So(measurements[0].Symbol, ShouldEqual, "AAA/USD")
			So(measurements[0].Source, ShouldEqual, types.SourceHawkes)
		})

		Convey("It should count both marks rather than the buy side alone", func() {
			// Counted arrivals are (origin, horizon], so the first buy is the
			// observation origin itself and the later sell has to be inside the
			// window: a buy-only horizon would have dropped it.
			So(measurements[0].Sample(
				types.MetricEventCount, types.SideNone,
			).Raw, ShouldEqual, 2)
			So(measurements[0].Sample(
				types.MetricEventCount, types.SideSell,
			).Raw, ShouldEqual, 1)
		})

	})

	Convey("Given a symbol whose only trades are sells", t, func() {
		signal := &Signal{
			ctx:     context.Background(),
			process: excitation.NewProcess(),
			sample:  excitation.NewSample(),
		}
		start := time.Unix(1_700_006_000, 0).UTC()
		thesis := types.NewThesis(t.Context(), nil)

		thesis.Trades.Store("BBB/USD", []kraken.TradeData{
			{Symbol: "BBB/USD", Side: "sell", Timestamp: start},
			{Symbol: "BBB/USD", Side: "sell", Timestamp: start.Add(time.Second)},
		})

		measurements := signal.Measure(thesis)

		Convey("It should still measure the one-sided arrival process", func() {
			So(measurements, ShouldHaveLength, 1)
			So(measurements[0].Symbol, ShouldEqual, "BBB/USD")
			So(measurements[0].Sample(types.MetricSNR, types.SideNone).Normalized,
				ShouldBeNil)
		})
	})

	Convey("Given trades for two independent symbols", t, func() {
		signal := &Signal{
			ctx:     context.Background(),
			process: excitation.NewProcess(),
			sample:  excitation.NewSample(),
		}
		start := time.Unix(1_700_006_000, 0).UTC()
		thesis := types.NewThesis(t.Context(), nil)

		thesis.Trades.Store("AAA/USD", []kraken.TradeData{
			{Symbol: "AAA/USD", Side: "buy", Timestamp: start},
			{Symbol: "AAA/USD", Side: "sell", Timestamp: start.Add(time.Second)},
		})
		thesis.Trades.Store("BBB/USD", []kraken.TradeData{
			{Symbol: "BBB/USD", Side: "sell", Timestamp: start},
			{Symbol: "BBB/USD", Side: "buy", Timestamp: start.Add(time.Second)},
		})

		measurements := signal.Measure(thesis)

		Convey("It should emit one row per symbol, never one per mark", func() {
			So(measurements, ShouldHaveLength, 2)
		})

		Convey("It should keep each symbol's estimator state apart", func() {
			So(signal.process.Symbols(), ShouldResemble,
				[]string{"AAA/USD", "BBB/USD"})
		})
	})

	Convey("Given a thesis carrying no trades", t, func() {
		signal := &Signal{
			ctx:     context.Background(),
			process: excitation.NewProcess(),
			sample:  excitation.NewSample(),
		}

		Convey("It should measure nothing", func() {
			So(func() []*types.Measurement {
				measurements := signal.Measure(types.NewThesis(t.Context(), nil))
				return measurements
			}(), ShouldBeEmpty)
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

func BenchmarkMeasure(t *testing.B) {
	signal := &Signal{
		ctx:     context.Background(),
		process: excitation.NewProcess(),
		sample:  excitation.NewSample(),
	}
	thesis := types.NewThesis(context.Background(), nil)
	start := time.Unix(1_700_005_000, 0).UTC()
	iteration := 0

	t.ReportAllocs()

	for t.Loop() {
		side := "buy"

		if iteration%2 != 0 {
			side = "sell"
		}

		thesis.Trades.Store("BTC/USD", []kraken.TradeData{{
			Symbol:    "BTC/USD",
			TradeID:   int64(iteration + 1),
			Side:      side,
			Timestamp: start.Add(time.Duration(iteration) * time.Second),
		}})
		signal.Measure(thesis)
		iteration++
	}
}
