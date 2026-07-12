package hawkes

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/algorithm/excitation"
	"github.com/theapemachine/nomagique/hawkes"
	"github.com/theapemachine/symm/types"
)

func TestEvidenceMeasure(t *testing.T) {
	Convey("Given an empirical arrival-rate outcome before model readiness", t, func() {
		evidence := NewEvidence()
		outcome := evidenceOutcome()
		outcome.Readiness = excitation.Readiness{
			Observation: true,
			Intensity:   true,
			Reason:      "fit pending",
		}
		measurements := evidence.Measure("BTC/USD", outcome)

		Convey("It should separate empirical rates from Hawkes baseline parameters", func() {
			So(measurements, ShouldHaveLength, 5)
			So(countMetric(measurements, types.MetricArrivalRate), ShouldEqual, 2)
			So(countMetric(measurements, types.MetricBaselineIntensity), ShouldEqual, 0)
		})
	})

	Convey("Given an identified but not forecast-validated Hawkes model", t, func() {
		evidence := NewEvidence()
		outcome := evidenceOutcome()
		outcome.Readiness = excitation.Readiness{
			Observation:  true,
			Intensity:    true,
			HawkesFit:    true,
			ModelUpdated: true,
			Reason:       "forecast validation pending",
		}
		measurements := evidence.Measure("BTC/USD", outcome)

		Convey("It should retain dimensions and explicit model limitations", func() {
			for _, measurement := range measurements {
				So(measurement.Validate(), ShouldBeNil)
			}

			for _, measurement := range measurements {
				if measurement.Validity.Readiness != types.ReadinessModel {
					continue
				}

				So(measurement.Validity.State, ShouldEqual,
					types.ValidityProvisional)
				So(measurement.Validity.Reason, ShouldNotBeEmpty)
			}
		})
	})

	Convey("Given a current intensity projected from a retained fit", t, func() {
		evidence := NewEvidence()
		outcome := evidenceOutcome()
		outcome.At = outcome.At.Add(time.Second)
		outcome.Horizon = outcome.At.Sub(outcome.ObservedFrom)
		outcome.Readiness = excitation.Readiness{
			Observation: true,
			Intensity:   true,
			HawkesFit:   true,
			Reason:      "retained fit",
		}
		measurements := evidence.Measure("BTC/USD", outcome)

		Convey("It should emit current intensities without repeating model parameters", func() {
			So(countMetric(measurements, types.MetricConditionalIntensity), ShouldEqual, 2)
			So(countMetric(measurements, types.MetricBaselineIntensity), ShouldEqual, 0)
			So(countMetric(measurements, types.MetricExcitationAmplitude), ShouldEqual, 0)

			for _, measurement := range measurements {
				So(measurement.Validate(), ShouldBeNil)

				if measurement.Metric == types.MetricConditionalIntensity {
					So(measurement.Scale.Through, ShouldResemble, outcome.FitAt)
				}
			}
		})
	})
}

func evidenceOutcome() excitation.Outcome {
	observedFrom := time.Date(2026, 7, 12, 2, 0, 0, 0, time.UTC)
	at := observedFrom.Add(time.Second)

	return excitation.Outcome{
		Fit: hawkes.BivariateFit{
			MuX:            1,
			MuY:            1,
			Beta:           2,
			IntensityX:     1,
			IntensityY:     1,
			SpectralRadius: 0,
		},
		ObservedFrom:    observedFrom,
		At:              at,
		Horizon:         time.Second,
		FitObservedFrom: observedFrom,
		FitAt:           at,
		EventCount:      2,
		BuyEventCount:   1,
		SellEventCount:  1,
		BuyArrivalRate:  1,
		SellArrivalRate: 1,
		Maturity:        1,
	}
}
