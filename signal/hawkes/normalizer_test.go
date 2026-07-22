package hawkes

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/algorithm/excitation"
	"github.com/theapemachine/nomagique/hawkes"
	"github.com/theapemachine/symm/types"
)

/*
TestNormalizer_Value proves normalization uses only compatible same-process
references at the readiness level the estimator has actually reached.
*/
func TestNormalizer_Value(t *testing.T) {
	Convey("Given an empirical marked arrival process", t, func() {
		outcome := excitation.Outcome{
			BuyArrivalRate:  3,
			SellArrivalRate: 1,
		}
		value := normalizer{}.value(
			outcome,
			types.MetricArrivalRate,
			types.SideBuy,
			types.UnitEventsPerSecond,
			outcome.BuyArrivalRate,
		)

		Convey("It should use the empirical two-sided mean before fitting", func() {
			So(value, ShouldResemble,
				types.NormalizeDeviation(outcome.BuyArrivalRate, 2))
		})
	})

	Convey("Given an identified Hawkes process", t, func() {
		outcome := excitation.Outcome{
			Readiness: excitation.Readiness{HawkesFit: true},
			Fit: hawkes.BivariateFit{
				MuX: 1,
				MuY: 2,
			},
		}

		Convey("It should use the fitted same-side baseline", func() {
			value := normalizer{}.value(
				outcome,
				types.MetricConditionalIntensity,
				types.SideSell,
				types.UnitEventsPerSecond,
				3,
			)

			So(value, ShouldResemble, types.NormalizeDeviation(3, outcome.Fit.MuY))
		})

		Convey("It should leave incompatible units explicit", func() {
			So(normalizer{}.value(
				outcome,
				types.MetricEventCount,
				types.SideNone,
				types.UnitCount,
				3,
			), ShouldBeNil)
		})
	})
}
