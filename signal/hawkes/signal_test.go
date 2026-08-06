package hawkes

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/algorithm/excitation"
	nomagiquehawkes "github.com/theapemachine/nomagique/hawkes"
)

func TestMeasurements(t *testing.T) {
	Convey("Given a retained fit evaluated on a later observation epoch", t, func() {
		fitFrom := time.Unix(1_700_001_000, 0).UTC()
		fitAt := fitFrom.Add(10 * time.Second)
		observedFrom := fitAt.Add(time.Second)
		at := observedFrom.Add(5 * time.Second)
		signal := &Signal{}
		measurements := signal.measurements("ALT/USD", excitationOutcome(
			fitFrom, fitAt, observedFrom, at,
		))
		measurement := measurements[0]

		Convey("It should keep evaluation provenance separate from fit scale", func() {
			So(measurement.ObservedFrom, ShouldResemble, observedFrom)
			So(measurement.At, ShouldResemble, at)
			So(measurement.Horizon, ShouldEqual, 5*time.Second)
		})
	})
}

func excitationOutcome(
	fitFrom, fitAt, observedFrom, at time.Time,
) excitation.Outcome {
	return excitation.Outcome{
		ObservedFrom:    observedFrom,
		At:              at,
		FitObservedFrom: fitFrom,
		FitAt:           fitAt,
		EventCount:      8,
		BuyEventCount:   4,
		SellEventCount:  4,
		BuyArrivalRate:  0.8,
		SellArrivalRate: 0.8,
		Maturity:        1,
		Fit: nomagiquehawkes.BivariateFit{
			MuX: 0.5, MuY: 0.5, Beta: 1, SpectralRadius: 0,
		},
		Readiness: excitation.Readiness{
			Observation: true,
			Intensity:   true,
			HawkesFit:   true,
			Reason:      "residual validation pending",
		},
	}
}
