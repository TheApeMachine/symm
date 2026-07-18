package hawkes

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/algorithm/excitation"
	"github.com/theapemachine/nomagique/hawkes"
	"github.com/theapemachine/symm/types"
)

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
		Fit: hawkes.BivariateFit{
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
	evidence := NewEvidence()
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
	evidence := NewEvidence()
	outcome := projectedOutcome()

	Convey("Given a retained Hawkes fit evaluated after its parameter epoch", t, func() {
		measurements := evidence.Measure("BTC/USD", outcome)
		model, ok := findMeasurement(measurements, types.MetricSpectralRadius)
		conditional, hasConditional := findMeasurement(
			measurements, types.MetricConditionalIntensity,
		)

		Convey("It should publish fit parameters beside live intensity", func() {
			So(ok, ShouldBeTrue)
			So(hasConditional, ShouldBeTrue)
			So(outcome.Readiness.ModelUpdated, ShouldBeFalse)
			So(model.ObservedFrom, ShouldEqual, outcome.FitObservedFrom)
			So(model.At, ShouldEqual, outcome.FitAt)
			So(model.Scale.From, ShouldEqual, outcome.FitObservedFrom)
			So(model.Scale.Through, ShouldEqual, outcome.FitAt)
			So(conditional.At, ShouldEqual, outcome.At)
			So(conditional.Scale.From, ShouldEqual, outcome.FitObservedFrom)
		})
	})
}

func TestEvidenceFitEpochBoundsFallback(t *testing.T) {
	evidence := NewEvidence()
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
