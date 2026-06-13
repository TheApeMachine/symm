package prediction

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/logic"
)

func TestPredictionRegimeShifted(t *testing.T) {
	Convey("Given an untagged origin regime", t, func() {
		origin := predictionRegime{}
		current := predictionRegime{
			source:   logic.SourceCausal,
			category: logic.CategoryEndogenousAlpha,
			ready:    true,
		}

		Convey("It should treat missing tags as shifted", func() {
			So(origin.Shifted(current), ShouldBeTrue)
		})
	})

	Convey("Given matching panic categories", t, func() {
		origin := predictionRegime{
			category: logic.CategoryLiquidityShock,
			ready:    true,
		}
		current := predictionRegime{
			category: logic.CategoryLiquidityShock,
			ready:    true,
		}

		Convey("It should not report a panic shift", func() {
			So(origin.Panic(), ShouldBeTrue)
			So(origin.Panic() == current.Panic(), ShouldBeTrue)
		})
	})
}
