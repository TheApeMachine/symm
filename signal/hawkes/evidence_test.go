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
evidenceIdentity adds the symbol whose marked process produced the quantity.
*/
type evidenceIdentity struct {
	evidenceKey
	symbol string
}

/*
evidenceValues keeps measurements addressable by their full identity.
*/
type evidenceValues map[evidenceIdentity]*types.Measurement

/*
marketOutcome retains transition peaks, final evidence, and readiness batches.
*/
type marketOutcome struct {
	peak    evidenceValues
	latest  evidenceValues
	batches []evidenceValues
	rows    []int
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
	latest := make(evidenceValues)
	rows := 0

	for _, measurement := range measurements {
		if measurement == nil || measurement.Source != types.SourceHawkes {
			continue
		}

		identity := evidenceIdentity{
			evidenceKey: evidenceKey{measurement.Metric, measurement.Side},
			symbol:      measurement.Symbol,
		}
		latest[identity] = measurement
		rows++

		peak, found := outcome.peak[identity]

		if !found || math.Abs(measurement.Raw) > math.Abs(peak.Raw) {
			outcome.peak[identity] = measurement
		}
	}

	outcome.latest = latest
	outcome.batches = append(outcome.batches, latest)
	outcome.rows = append(outcome.rows, rows)
}

/*
Value returns one exact metric, side, and symbol without merging marked rows.
*/
func (values evidenceValues) Value(
	metric types.MetricType,
	side types.MeasurementSide,
	symbol string,
) float64 {
	return values[evidenceIdentity{
		evidenceKey: evidenceKey{metric, side},
		symbol:      symbol,
	}].Raw
}

/*
Prove validates every metric and side at the transition peak and final batch.
*/
func (outcome marketOutcome) Prove(symbols []string, fitted bool) {
	expected := len(evidenceKeys) * len(symbols)
	So(outcome.peak, ShouldHaveLength, expected)
	latestExpected := len(evidenceKeys[:5]) * len(symbols)

	if fitted {
		latestExpected = expected
	}

	So(outcome.latest, ShouldHaveLength, latestExpected)

	for index, batch := range outcome.batches {
		So(outcome.rows[index], ShouldEqual, len(batch))
	}

	for _, key := range evidenceKeys {
		for _, symbol := range symbols {
			identity := evidenceIdentity{evidenceKey: key, symbol: symbol}
			values := []evidenceValues{outcome.peak}

			if fitted || key.metric == types.MetricEventCount ||
				key.metric == types.MetricArrivalRate {
				values = append(values, outcome.latest)
			}

			for _, values := range values {
				measurement, found := values[identity]
				So(found, ShouldBeTrue)
				So(measurement.ValidateStruct(), ShouldBeNil)
				So(math.IsNaN(measurement.Raw), ShouldBeFalse)
				So(math.IsInf(measurement.Raw, 0), ShouldBeFalse)
				So(measurement.Maturity, ShouldBeBetweenOrEqual, 0.0, 1.0)

				if key.metric != types.MetricHawkesPoissonDelta &&
					key.metric != types.MetricCrossSelfDelta {
					So(measurement.Raw, ShouldBeGreaterThanOrEqualTo, 0.0)
				}

				if key.metric == types.MetricEventCount {
					So(measurement.Validity.State, ShouldEqual, types.ValidityValid)
					So(measurement.Validity.Readiness,
						ShouldEqual, types.ReadinessObservation)
				}

				if key.metric == types.MetricArrivalRate {
					So(measurement.Validity.State, ShouldEqual, types.ValidityValid)
					So(measurement.Validity.Readiness,
						ShouldEqual, types.ReadinessIntensity)
					So(measurement.Raw, ShouldBeGreaterThan, 0.0)
				}

				if key.metric != types.MetricEventCount &&
					key.metric != types.MetricArrivalRate {
					So(measurement.Validity.State, ShouldEqual, types.ValidityProvisional)
					So(measurement.Validity.Readiness,
						ShouldEqual, types.ReadinessModel)
					So(measurement.Validity.Reason, ShouldNotBeBlank)
				}
			}
		}
	}

	for _, symbol := range symbols {
		So(outcome.latest.Value(types.MetricEventCount, types.SideNone, symbol),
			ShouldEqual,
			outcome.latest.Value(types.MetricEventCount, types.SideBuy, symbol)+
				outcome.latest.Value(types.MetricEventCount, types.SideSell, symbol))

		if !fitted {
			So(outcome.latest[evidenceIdentity{
				evidenceKey: evidenceKey{types.MetricArrivalRate, types.SideBuy},
				symbol:      symbol,
			}].Validity.Reason, ShouldContainSubstring, "per side")
			continue
		}

		So(outcome.latest.Value(types.MetricConditionalIntensity, types.SideBuy, symbol),
			ShouldBeGreaterThanOrEqualTo,
			outcome.latest.Value(types.MetricBaselineIntensity, types.SideBuy, symbol))
		So(outcome.latest.Value(types.MetricConditionalIntensity, types.SideSell, symbol),
			ShouldBeGreaterThanOrEqualTo,
			outcome.latest.Value(types.MetricBaselineIntensity, types.SideSell, symbol))
		So(outcome.latest.Value(types.MetricTotalDescendants, types.SideBuy, symbol),
			ShouldBeGreaterThanOrEqualTo,
			outcome.latest.Value(types.MetricImmediateOffspring, types.SideBuy, symbol))
		So(outcome.latest.Value(types.MetricTotalDescendants, types.SideSell, symbol),
			ShouldBeGreaterThanOrEqualTo,
			outcome.latest.Value(types.MetricImmediateOffspring, types.SideSell, symbol))
		So(outcome.latest.Value(types.MetricDecayRate, types.SideNone, symbol)*
			outcome.latest.Value(types.MetricKernelMemory, types.SideNone, symbol),
			ShouldAlmostEqual, 1.0)
		So(outcome.latest.Value(types.MetricSpectralRadius, types.SideNone, symbol),
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
			So(measurements, ShouldNotBeEmpty)

			for _, measurement := range measurements {
				So(measurement.ValidateStruct(), ShouldBeNil)
			}
		})

		Convey("It should derive a missing origin from the observed horizon", func() {
			missingOrigin := outcome
			missingOrigin.ObservedFrom = time.Time{}
			measurements := evidence.Measure("BTC/USD", missingOrigin)

			So(measurements, ShouldNotBeEmpty)
			So(measurements[0].ObservedFrom, ShouldEqual, outcome.At.Add(-outcome.Horizon))
		})
	})
}

func TestEvidenceModelEpochAnchoring(t *testing.T) {
	evidence := hawkessignal.NewEvidence()
	outcome := projectedOutcome()

	Convey("Given a retained Hawkes fit evaluated after its parameter epoch", t, func() {
		measurements := evidence.Measure("BTC/USD", outcome)
		model, ok := findMeasurement(measurements, types.MetricSpectralRadius)
		conditional, hasConditional := findMeasurement(
			measurements, types.MetricConditionalIntensity,
		)
		comparison, hasComparison := findMeasurement(
			measurements, types.MetricHawkesPoissonDelta,
		)

		Convey("It should distinguish fit epochs from live evaluations", func() {
			So(ok, ShouldBeTrue)
			So(hasConditional, ShouldBeTrue)
			So(hasComparison, ShouldBeTrue)
			So(outcome.Readiness.ModelUpdated, ShouldBeFalse)
			So(model.ObservedFrom, ShouldEqual, outcome.FitObservedFrom)
			So(model.At, ShouldEqual, outcome.FitAt)
			So(model.Scale.From, ShouldEqual, outcome.FitObservedFrom)
			So(model.Scale.Through, ShouldEqual, outcome.FitAt)
			So(conditional.At, ShouldEqual, outcome.At)
			So(conditional.Scale.From, ShouldEqual, outcome.FitObservedFrom)
			So(comparison.At, ShouldEqual, outcome.At)
			So(comparison.Scale.From, ShouldEqual, outcome.FitObservedFrom)
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
			validated := 0

			for _, measurement := range measurements {
				if measurement.Metric == types.MetricEventCount ||
					measurement.Metric == types.MetricArrivalRate {
					continue
				}

				So(measurement.ValidateStruct(), ShouldBeNil)
				validated++
			}

			So(validated, ShouldBeGreaterThan, 0)
			model, ok := findMeasurement(measurements, types.MetricSpectralRadius)
			So(ok, ShouldBeTrue)
			So(model.ObservedFrom, ShouldEqual, outcome.FitAt.Add(-outcome.Horizon))
		})
	})
}

func findMeasurement(
	measurements []*types.Measurement,
	metric types.MetricType,
) (*types.Measurement, bool) {
	for _, measurement := range measurements {
		if measurement.Metric == metric {
			return measurement, true
		}
	}

	return nil, false
}
