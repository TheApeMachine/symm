package hawkes

import (
	"context"
	"math"
	"sync"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/algorithm/excitation"
	nmhawkes "github.com/theapemachine/nomagique/hawkes"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

/*
excitationOutcome builds an outcome whose fit epoch precedes the evaluation
epoch, so provenance and scale can be told apart in assertions.
*/
func excitationOutcome(fitted bool) excitation.Outcome {
	fitFrom := time.Unix(1_700_001_000, 0).UTC()
	observedFrom := fitFrom.Add(11 * time.Second)

	return excitation.Outcome{
		ObservedFrom:                    observedFrom,
		At:                              observedFrom.Add(5 * time.Second),
		FitObservedFrom:                 fitFrom,
		FitAt:                           fitFrom.Add(10 * time.Second),
		EventCount:                      8,
		BuyEventCount:                   6,
		SellEventCount:                  2,
		BuyArrivalRate:                  1.2,
		SellArrivalRate:                 0.4,
		MinimumFitEvents:                16,
		Maturity:                        1,
		HawkesPoissonLogLikelihoodDelta: 4,
		CrossSelfLogLikelihoodDelta:     2,
		ImmediateBuyOffspring:           0.5,
		TotalBuyDescendants:             1.25,
		Fit: nmhawkes.BivariateFit{
			MuX: 0.5, MuY: 0.5,
			AlphaXX: 1, AlphaXY: 0.5, AlphaYX: 0.25, AlphaYY: 0.75,
			Beta: 2, SpectralRadius: 0.4,
			IntensityX: 2, IntensityY: 0.25,
		},
		Readiness: excitation.Readiness{
			Observation: true,
			Intensity:   true,
			HawkesFit:   fitted,
			Reason:      "residual validation pending",
		},
	}
}

func TestMeasure(t *testing.T) {
	Convey("Given a liquid market and an unrelated thin market", t, func() {
		signal := &Signal{ctx: context.Background(), processors: &sync.Map{}}
		start := time.Unix(1_700_006_000, 0).UTC()
		thesis := types.NewThesis(nil)
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

		_, ready := signal.measure(thesis)

		Convey("It should not let the thin market veto Hawkes readiness", func() {
			So(ready, ShouldBeTrue)
		})
	})

	Convey("Given trades for symbols whose marks arrive out of order", t, func() {
		signal := &Signal{ctx: context.Background(), processors: &sync.Map{}}
		start := time.Unix(1_700_006_000, 0).UTC()
		thesis := types.NewThesis(nil)

		// NewArrivalStream sorts each side, so the signal must not pre-sort. The
		// sell here lands after the last buy, which is what makes the horizon
		// span both marks rather than the buy side alone.
		thesis.Trades.Store("AAA/USD", []kraken.TradeData{
			{Symbol: "AAA/USD", Side: "sell", Timestamp: start.Add(2 * time.Second)},
			{Symbol: "AAA/USD", Side: "buy", Timestamp: start},
			{Symbol: "AAA/USD", Side: "buy", Timestamp: start.Add(time.Second)},
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
		signal := &Signal{ctx: context.Background(), processors: &sync.Map{}}
		start := time.Unix(1_700_006_000, 0).UTC()
		thesis := types.NewThesis(nil)

		thesis.Trades.Store("BBB/USD", []kraken.TradeData{
			{Symbol: "BBB/USD", Side: "sell", Timestamp: start},
			{Symbol: "BBB/USD", Side: "sell", Timestamp: start.Add(time.Second)},
		})

		measurements := signal.Measure(thesis)

		Convey("It should still measure the one-sided arrival process", func() {
			So(measurements, ShouldHaveLength, 1)
			So(measurements[0].Symbol, ShouldEqual, "BBB/USD")
		})
	})

	Convey("Given trades for two independent symbols", t, func() {
		signal := &Signal{ctx: context.Background(), processors: &sync.Map{}}
		start := time.Unix(1_700_006_000, 0).UTC()
		thesis := types.NewThesis(nil)

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
			for _, symbol := range []string{"AAA/USD", "BBB/USD"} {
				process, ok := signal.processors.Load(symbol)

				So(ok, ShouldBeTrue)
				So(process.(*excitation.Process).Symbols(), ShouldResemble, []string{symbol})
			}
		})
	})

	Convey("Given a thesis carrying no trades", t, func() {
		signal := &Signal{ctx: context.Background(), processors: &sync.Map{}}

		Convey("It should measure nothing", func() {
			So(signal.Measure(types.NewThesis(nil)), ShouldBeEmpty)
		})
	})
}

func TestMeasurement(t *testing.T) {
	Convey("Given an identified fit evaluated on a later observation epoch", t, func() {
		signal := &Signal{}
		outcome := excitationOutcome(true)
		measurement := signal.measurement("ALT/USD", outcome)

		Convey("It should keep evaluation provenance separate from fit scale", func() {
			So(measurement.ObservedFrom, ShouldResemble, outcome.ObservedFrom)
			So(measurement.At, ShouldResemble, outcome.At)
			So(measurement.Horizon, ShouldEqual, 5*time.Second)
		})

		Convey("It should report self-excitation against the immigrant baseline", func() {
			// Intensity 2.0 over a baseline of 0.5 is three baselines of excitation.
			sample := measurement.Sample(
				types.MetricConditionalIntensity, types.SideBuy,
			)

			So(sample.Normalized, ShouldNotBeNil)
			So(*sample.Normalized, ShouldAlmostEqual, 3.0, 1e-9)
		})

		Convey("It should refuse an intensity beneath its own baseline", func() {
			// A nonnegative kernel cannot hold IntensityY 0.25 under MuY 0.5.
			So(measurement.Sample(
				types.MetricConditionalIntensity, types.SideSell,
			).Normalized, ShouldBeNil)
		})

		Convey("It should scale each amplitude by the decay consuming it", func() {
			So(*measurement.Sample(
				types.MetricExcitationAmplitude, types.SideBuyToBuy,
			).Normalized, ShouldAlmostEqual, 0.5, 1e-9)
		})

		Convey("It should publish kernel memory as a share of the horizon", func() {
			sample := measurement.Sample(types.MetricKernelMemory, types.SideNone)

			So(sample.Raw, ShouldAlmostEqual, 0.5, 1e-9)
			So(*sample.Normalized, ShouldAlmostEqual, 0.1, 1e-9)
		})

		Convey("It should report likelihood gains per observed event", func() {
			So(*measurement.Sample(
				types.MetricHawkesPoissonDelta, types.SideNone,
			).Normalized, ShouldAlmostEqual, 0.5, 1e-9)
		})

		Convey("It should mark the row as carrying fit parameters", func() {
			So(types.ForPublish([]*types.Measurement{measurement}), ShouldHaveLength, 1)
		})
	})

	Convey("Given an outcome whose fit is not identifiable", t, func() {
		signal := &Signal{}
		measurement := signal.measurement("ALT/USD", excitationOutcome(false))

		Convey("It should omit fitted state rather than publish a zero", func() {
			for _, metric := range []types.MetricType{
				types.MetricConditionalIntensity,
				types.MetricBaselineIntensity,
				types.MetricSpectralRadius,
				types.MetricDecayRate,
			} {
				_, present := measurement.Metrics[types.MetricKey(metric, types.SideBuy)]
				So(present, ShouldBeFalse)

				_, present = measurement.Metrics[types.MetricKey(metric, types.SideNone)]
				So(present, ShouldBeFalse)
			}
		})

		Convey("It should still publish the empirical arrival rate", func() {
			sample := measurement.Sample(types.MetricArrivalRate, types.SideBuy)

			So(sample.Raw, ShouldAlmostEqual, 1.2, 1e-9)
			So(sample.Unit, ShouldEqual, types.UnitEventsPerSecond)
		})

		Convey("It should scale rates against the total marked rate", func() {
			// 1.2 buys against 1.6 marked arrivals per second.
			So(*measurement.Sample(
				types.MetricArrivalRate, types.SideBuy,
			).Normalized, ShouldAlmostEqual, 0.75, 1e-9)
		})

		Convey("It should measure support against the estimator requirement", func() {
			So(*measurement.Sample(
				types.MetricEventCount, types.SideNone,
			).Normalized, ShouldAlmostEqual, 0.5, 1e-9)
		})
	})
}

func TestNormalizedShare(t *testing.T) {
	Convey("Given a reading and a reference scale", t, func() {
		Convey("It should report the reading as a fraction of the reference", func() {
			So(*normalizedShare(3, 4), ShouldAlmostEqual, 0.75, 1e-9)
		})

		Convey("It should refuse a reference that establishes no scale", func() {
			So(normalizedShare(3, 0), ShouldBeNil)
			So(normalizedShare(3, -1), ShouldBeNil)
			So(normalizedShare(math.NaN(), 4), ShouldBeNil)
			So(normalizedShare(math.Inf(1), 4), ShouldBeNil)
		})
	})
}

func TestNormalizedBranching(t *testing.T) {
	Convey("Given a fitted branching ratio", t, func() {
		Convey("It should publish a stationary process", func() {
			So(*normalizedBranching(0.4), ShouldAlmostEqual, 0.4, 1e-9)
		})

		Convey("It should refuse a ratio whose cascade size diverges", func() {
			So(normalizedBranching(criticalBranch), ShouldBeNil)
			So(normalizedBranching(1.5), ShouldBeNil)
			So(normalizedBranching(-0.1), ShouldBeNil)
		})
	})
}
