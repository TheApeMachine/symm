package hawkes

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/algorithm/excitation"
	nomagiquehawkes "github.com/theapemachine/nomagique/hawkes"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

func TestSeenTrade(t *testing.T) {
	Convey("Given an exact-once cursor for one symbol", t, func() {
		signal := &Signal{lastTrade: make(map[string]tradeCursor)}
		at := time.Unix(1_700_000_000, 0).UTC()
		first := kraken.TradeData{Symbol: "ALT/USD", TradeID: 11, Timestamp: at}
		secondSameTime := kraken.TradeData{Symbol: "ALT/USD", TradeID: 12, Timestamp: at}
		regressed := kraken.TradeData{
			Symbol: "ALT/USD", TradeID: 13, Timestamp: at.Add(-time.Nanosecond),
		}

		Convey("It should accept distinct same-time IDs and reject replay or regression", func() {
			So(signal.seenTrade(first), ShouldBeFalse)
			signal.commitTrade(first)
			So(signal.seenTrade(first), ShouldBeTrue)
			So(signal.seenTrade(secondSameTime), ShouldBeFalse)
			signal.commitTrade(secondSameTime)
			So(signal.seenTrade(secondSameTime), ShouldBeTrue)
			So(signal.seenTrade(regressed), ShouldBeTrue)
		})
	})

	Convey("Given same-time trades without exchange IDs", t, func() {
		signal := &Signal{lastTrade: make(map[string]tradeCursor)}
		at := time.Unix(1_700_000_100, 0).UTC()
		unidentified := kraken.TradeData{Symbol: "ALT/USD", Timestamp: at}

		signal.commitTrade(unidentified)

		Convey("It should document the intrinsic indistinguishability by rejecting the second zero-ID event", func() {
			So(signal.seenTrade(unidentified), ShouldBeTrue)
		})
	})
}

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
			So(measurement.Scale.From, ShouldResemble, fitFrom)
			So(measurement.Scale.Through, ShouldResemble, fitAt)
			So(measurement.Validity.State, ShouldEqual, types.ValidityProvisional)
			So(measurement.Validity.Readiness, ShouldEqual, types.ReadinessModel)
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
