package hawkes_test

import (
	"math"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/algorithm/excitation"
	hawkesmodel "github.com/theapemachine/nomagique/hawkes"
	hawkessignal "github.com/theapemachine/symm/signal/hawkes"
	"github.com/theapemachine/symm/types"
)

/*
evidenceKey preserves the metric and marked side of one Hawkes quantity.
*/
type evidenceKey struct {
	metric types.MetricType
	side   types.MeasurementSide
}

/*
marketOutcome retains transition peaks, final evidence, and readiness batches.
*/
type marketOutcome struct {
	peak     map[evidenceKey]map[string]float64
	latest   map[string]*types.Measurement
	batches  []map[string]*types.Measurement
	rowCount []int
}

var evidenceKeys = []evidenceKey{
	{types.MetricEventCount, types.SideNone},
	{types.MetricEventCount, types.SideBuy},
	{types.MetricEventCount, types.SideSell},
	{types.MetricArrivalRate, types.SideBuy},
	{types.MetricArrivalRate, types.SideSell},
	{types.MetricConditionalIntensity, types.SideBuy},
	{types.MetricConditionalIntensity, types.SideSell},
	{types.MetricBaselineIntensity, types.SideBuy},
	{types.MetricBaselineIntensity, types.SideSell},
	{types.MetricExcitationAmplitude, types.SideBuyToBuy},
	{types.MetricExcitationAmplitude, types.SideSellToBuy},
	{types.MetricExcitationAmplitude, types.SideBuyToSell},
	{types.MetricExcitationAmplitude, types.SideSellToSell},
	{types.MetricDecayRate, types.SideNone},
	{types.MetricKernelMemory, types.SideNone},
	{types.MetricSpectralRadius, types.SideNone},
	{types.MetricHawkesPoissonDelta, types.SideNone},
	{types.MetricCrossSelfDelta, types.SideNone},
	{types.MetricImmediateOffspring, types.SideBuy},
	{types.MetricImmediateOffspring, types.SideSell},
	{types.MetricTotalDescendants, types.SideBuy},
	{types.MetricTotalDescendants, types.SideSell},
}

/*
Capture reduces one successful production tick without erasing side identity.
*/
func (outcome *marketOutcome) Capture(measurements []*types.Measurement) {
	latest := make(map[string]*types.Measurement)
	rows := 0

	for _, measurement := range measurements {
		if measurement == nil || measurement.Source != types.SourceHawkes {
			continue
		}

		latest[measurement.Symbol] = measurement
		rows++

		for _, key := range evidenceKeys {
			sample, ok := measurement.Sample(key.metric, key.side)

			if !ok {
				continue
			}

			if outcome.peak == nil {
				outcome.peak = map[evidenceKey]map[string]float64{}
			}

			bySymbol, found := outcome.peak[key]

			if !found {
				bySymbol = map[string]float64{}
				outcome.peak[key] = bySymbol
			}

			peak, seen := bySymbol[measurement.Symbol]

			if !seen || math.Abs(sample.Raw) > math.Abs(peak) {
				bySymbol[measurement.Symbol] = sample.Raw
			}
		}
	}

	if rows == 0 {
		return
	}

	outcome.latest = latest
	outcome.batches = append(outcome.batches, latest)
	outcome.rowCount = append(outcome.rowCount, rows)
}

/*
Value returns one exact metric, side, and symbol without merging marked rows.
*/
func (outcome marketOutcome) Value(
	metric types.MetricType,
	side types.MeasurementSide,
	symbol string,
) float64 {
	measurement := outcome.latest[symbol]

	if measurement == nil {
		return 0
	}

	sample, ok := measurement.Sample(metric, side)

	if !ok {
		return 0
	}

	return sample.Raw
}

/*
Prove validates every metric and side at the transition peak and final batch.
*/
func (outcome marketOutcome) Prove(symbols []string, fitted bool) {
	for _, key := range evidenceKeys {
		So(outcome.peak[key], ShouldHaveLength, len(symbols))
	}

	for index, batch := range outcome.batches {
		So(outcome.rowCount[index], ShouldEqual, len(batch))
		So(batch, ShouldHaveLength, len(symbols))
	}

	for _, symbol := range symbols {
		measurement := outcome.latest[symbol]
		So(measurement, ShouldNotBeNil)
		So(measurement.ValidateStruct(), ShouldBeNil)
		So(measurement.Maturity, ShouldBeBetweenOrEqual, 0.0, 1.0)

		for _, key := range evidenceKeys {
			sample, found := measurement.Sample(key.metric, key.side)
			So(found, ShouldBeTrue)
			So(math.IsNaN(sample.Raw), ShouldBeFalse)
			So(math.IsInf(sample.Raw, 0), ShouldBeFalse)

			if key.metric != types.MetricHawkesPoissonDelta &&
				key.metric != types.MetricCrossSelfDelta {
				So(sample.Raw, ShouldBeGreaterThanOrEqualTo, 0.0)
			}

			if key.metric == types.MetricArrivalRate &&
				measurement.Validity.State == types.ValidityValid {
				So(sample.Raw, ShouldBeGreaterThan, 0.0)
			}
		}

		So(outcome.Value(types.MetricEventCount, types.SideNone, symbol),
			ShouldEqual,
			outcome.Value(types.MetricEventCount, types.SideBuy, symbol)+
				outcome.Value(types.MetricEventCount, types.SideSell, symbol))

		if !fitted {
			So(measurement.Validity.State, ShouldEqual, types.ValidityProvisional)
			So(measurement.Validity.Reason, ShouldContainSubstring, "per side")
			So(outcome.Value(
				types.MetricConditionalIntensity, types.SideBuy, symbol,
			), ShouldEqual, 0)
			continue
		}

		So(outcome.Value(types.MetricConditionalIntensity, types.SideBuy, symbol),
			ShouldBeGreaterThanOrEqualTo,
			outcome.Value(types.MetricBaselineIntensity, types.SideBuy, symbol))
		So(outcome.Value(types.MetricConditionalIntensity, types.SideSell, symbol),
			ShouldBeGreaterThanOrEqualTo,
			outcome.Value(types.MetricBaselineIntensity, types.SideSell, symbol))
		So(outcome.Value(types.MetricTotalDescendants, types.SideBuy, symbol),
			ShouldBeGreaterThanOrEqualTo,
			outcome.Value(types.MetricImmediateOffspring, types.SideBuy, symbol))
		So(outcome.Value(types.MetricTotalDescendants, types.SideSell, symbol),
			ShouldBeGreaterThanOrEqualTo,
			outcome.Value(types.MetricImmediateOffspring, types.SideSell, symbol))
		So(outcome.Value(types.MetricDecayRate, types.SideNone, symbol)*
			outcome.Value(types.MetricKernelMemory, types.SideNone, symbol),
			ShouldAlmostEqual, 1.0)
		So(outcome.Value(types.MetricSpectralRadius, types.SideNone, symbol),
			ShouldBeLessThan, 1.0)
	}
}

func projectedOutcome() excitation.Outcome {
	fitAt := time.Unix(100, 0)
	observedFrom := fitAt.Add(30 * time.Second)
	at := observedFrom.Add(10 * time.Second)

	return excitation.Outcome{
		At:              at,
		ObservedFrom:    observedFrom,
		Horizon:         at.Sub(observedFrom),
		FitAt:           fitAt,
		FitObservedFrom: fitAt.Add(-20 * time.Second),
		EventCount:      12,
		BuyEventCount:   7,
		SellEventCount:  5,
		Maturity:        1,
		Readiness: excitation.Readiness{
			Observation: true,
			Intensity:   true,
			HawkesFit:   true,
		},
		Fit: hawkesmodel.BivariateFit{
			MuX:        1,
			MuY:        1,
			AlphaXX:    0.2,
			AlphaYY:    0.2,
			Beta:       2,
			IntensityX: 1.5,
			IntensityY: 0.8,
		},
	}
}

func TestEvidenceMeasureProjectedIntervals(t *testing.T) {
	evidence := hawkessignal.NewEvidence()
	outcome := projectedOutcome()

	Convey("Given a retained Hawkes fit evaluated on a newer trade window", t, func() {
		measurements := evidence.Measure("BTC/USD", outcome)

		Convey("It should publish forward evidence intervals for every metric", func() {
			So(measurements, ShouldHaveLength, 1)
			So(measurements[0].ValidateStruct(), ShouldBeNil)
			So(measurements[0].Metrics, ShouldHaveLength, len(evidenceKeys))
		})

		Convey("It should derive a missing origin from the observed horizon", func() {
			missingOrigin := outcome
			missingOrigin.ObservedFrom = time.Time{}
			measurements := evidence.Measure("BTC/USD", missingOrigin)

			So(measurements, ShouldHaveLength, 1)
			So(measurements[0].ObservedFrom, ShouldEqual, outcome.FitObservedFrom)
		})
	})
}

func TestEvidenceModelEpochAnchoring(t *testing.T) {
	evidence := hawkessignal.NewEvidence()
	outcome := projectedOutcome()

	Convey("Given a retained Hawkes fit evaluated after its parameter epoch", t, func() {
		measurements := evidence.Measure("BTC/USD", outcome)
		measurement := measurements[0]
		_, hasSpectral := measurement.Sample(types.MetricSpectralRadius, types.SideNone)
		conditional, hasConditional := measurement.Sample(
			types.MetricConditionalIntensity, types.SideBuy,
		)
		comparison, hasComparison := measurement.Sample(
			types.MetricHawkesPoissonDelta, types.SideNone,
		)

		Convey("It should distinguish fit epochs from live evaluations", func() {
			So(hasSpectral, ShouldBeTrue)
			So(hasConditional, ShouldBeTrue)
			So(hasComparison, ShouldBeTrue)
			So(outcome.Readiness.ModelUpdated, ShouldBeFalse)
			So(measurement.At, ShouldEqual, outcome.At)
			So(measurement.Scale.From, ShouldEqual, outcome.FitObservedFrom)
			So(measurement.Scale.Through, ShouldEqual, outcome.At)
			So(conditional.Raw, ShouldBeGreaterThan, 0)
			So(comparison.Raw, ShouldEqual, outcome.HawkesPoissonLogLikelihoodDelta)
		})
	})
}

func TestEvidenceModelEpochFallback(t *testing.T) {
	evidence := hawkessignal.NewEvidence()
	outcome := projectedOutcome()
	outcome.FitObservedFrom = time.Time{}
	outcome.ObservedFrom = time.Time{}
	outcome.Readiness.ModelUpdated = true

	Convey("Given a fit epoch missing its origin stamp", t, func() {
		measurements := evidence.Measure("BTC/USD", outcome)

		Convey("It should still publish forward model intervals", func() {
			So(measurements, ShouldHaveLength, 1)
			So(measurements[0].ValidateStruct(), ShouldBeNil)
			So(measurements[0].ObservedFrom,
				ShouldEqual, outcome.At.Add(-outcome.Horizon))
			_, ok := measurements[0].Sample(types.MetricSpectralRadius, types.SideNone)
			So(ok, ShouldBeTrue)
		})
	})
}
