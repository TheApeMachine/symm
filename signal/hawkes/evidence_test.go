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
			for _, measurement := range measurements {
				So(measurement.ValidateStruct(), ShouldBeNil)
			}
		})
	})
}

func TestEvidenceModelEpochAnchoring(t *testing.T) {
	evidence := NewEvidence()
	outcome := projectedOutcome()
	outcome.Readiness.ModelUpdated = true

	Convey("Given a model-updated Hawkes outcome", t, func() {
		measurements := evidence.Measure("BTC/USD", outcome)
		model, ok := findMeasurement(measurements, types.MetricSpectralRadius)

		Convey("It should anchor parameter evidence to the fit epoch", func() {
			So(ok, ShouldBeTrue)
			So(model.ObservedFrom, ShouldEqual, outcome.FitObservedFrom)
			So(model.At, ShouldEqual, outcome.FitAt)
			So(model.Scale.From, ShouldEqual, outcome.FitObservedFrom)
			So(model.Scale.Through, ShouldEqual, outcome.FitAt)
		})
	})
}

func TestEvidenceFitEpochBoundsFallback(t *testing.T) {
	evidence := NewEvidence()
	outcome := projectedOutcome()
	outcome.FitObservedFrom = time.Time{}
	outcome.Readiness.ModelUpdated = true

	Convey("Given a fit epoch missing its origin stamp", t, func() {
		measurements := evidence.Measure("BTC/USD", outcome)

		Convey("It should still publish forward model intervals", func() {
			for _, measurement := range measurements {
				if measurement.Metric == types.MetricEventCount ||
					measurement.Metric == types.MetricArrivalRate {
					continue
				}

				So(measurement.ValidateStruct(), ShouldBeNil)
			}
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
