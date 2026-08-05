package hawkes

import (
	"math"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/algorithm/excitation"
	nomagiquehawkes "github.com/theapemachine/nomagique/hawkes"
	"github.com/theapemachine/symm/types"
)

func TestValue(t *testing.T) {
	Convey("Given a ready Hawkes fit with a real observation scale", t, func() {
		outcome := normalizationOutcome()
		normalization := normalizer{}

		Convey("It should use the contract belonging to each metric", func() {
			buyShare := normalization.value(
				outcome, types.MetricEventCount, types.SideBuy, types.UnitCount, 6,
			)
			branching := normalization.value(
				outcome,
				types.MetricExcitationAmplitude,
				types.SideBuyToSell,
				types.UnitEventsPerSecond,
				0.4,
			)
			perEvent := normalization.value(
				outcome,
				types.MetricHawkesPoissonDelta,
				types.SideNone,
				types.UnitNat,
				6,
			)
			kernelShare := normalization.value(
				outcome,
				types.MetricKernelMemory,
				types.SideNone,
				types.UnitSecond,
				0.5,
			)

			So(*buyShare, ShouldAlmostEqual, 0.6, 1e-12)
			So(*branching, ShouldAlmostEqual, 0.2, 1e-12)
			So(*perEvent, ShouldAlmostEqual, 0.6, 1e-12)
			So(*kernelShare, ShouldAlmostEqual, 0.05, 1e-12)
		})

		Convey("It should preserve a genuine normalized zero", func() {
			value := normalization.value(
				outcome,
				types.MetricExcitationAmplitude,
				types.SideBuyToBuy,
				types.UnitEventsPerSecond,
				0,
			)

			So(value, ShouldNotBeNil)
			So(*value, ShouldEqual, 0.0)
		})
	})

	Convey("Given absent, malformed, or provisional normalization evidence", t, func() {
		outcome := normalizationOutcome()
		normalization := normalizer{}

		Convey("It should leave the normalized result absent", func() {
			outcome.MinimumFitEvents = 0
			So(normalization.value(
				outcome, types.MetricEventCount, types.SideNone, types.UnitCount, 10,
			), ShouldBeNil)

			outcome.Readiness.HawkesFit = false
			So(normalization.value(
				outcome,
				types.MetricExcitationAmplitude,
				types.SideBuyToBuy,
				types.UnitEventsPerSecond,
				0.2,
			), ShouldBeNil)

			So(normalization.value(
				outcome,
				types.MetricArrivalRate,
				types.SideBuy,
				types.UnitEventsPerSecond,
				math.NaN(),
			), ShouldBeNil)
		})
	})
}

func normalizationOutcome() excitation.Outcome {
	from := time.Unix(1_700_010_000, 0).UTC()

	return excitation.Outcome{
		ObservedFrom:     from,
		At:               from.Add(10 * time.Second),
		Horizon:          10 * time.Second,
		EventCount:       10,
		BuyEventCount:    6,
		SellEventCount:   4,
		BuyArrivalRate:   0.6,
		SellArrivalRate:  0.4,
		MinimumFitEvents: 8,
		Fit: nomagiquehawkes.BivariateFit{
			MuX: 0.3, MuY: 0.2, AlphaXX: 0, AlphaYX: 0.4,
			Beta: 2, SpectralRadius: 0.2,
		},
		Readiness: excitation.Readiness{
			Observation: true,
			Intensity:   true,
			HawkesFit:   true,
		},
	}
}

func BenchmarkValue(b *testing.B) {
	outcome := normalizationOutcome()
	normalization := normalizer{}

	b.ReportAllocs()

	for range b.N {
		_ = normalization.value(
			outcome,
			types.MetricExcitationAmplitude,
			types.SideBuyToSell,
			types.UnitEventsPerSecond,
			0.4,
		)
	}
}
