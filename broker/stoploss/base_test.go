package stoploss

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestStopLossDamageIgnoresEntryFriction(t *testing.T) {
	Convey("opening drag inside the arm hurdle is not catastrophic damage", t, func() {
		stop := NewStopLoss()
		stop.budget = NewBudget().Arm(0.02) // 2% friction → arm ≥ 2pp

		_, _ = stop.Update(-1.0)
		_, _ = stop.Update(-1.6)

		So(stop.damageRatio(), ShouldEqual, 0)
	})
}

func TestStopLossAllowedGivebackScalesWithPeak(t *testing.T) {
	Convey("giveback widens with peak instead of compressing to minGiveback", t, func() {
		stop := NewStopLoss()

		small := stop.allowedGiveback(2.0, 0)
		large := stop.allowedGiveback(40.0, 0)

		So(small, ShouldBeGreaterThanOrEqualTo, stop.minGiveback)
		So(large, ShouldBeGreaterThan, small)
		So(large, ShouldAlmostEqual, 40.0*trailGivebackFraction, 0.01)
	})
}

func TestStopLossTrailHoldsRunnerThroughNoise(t *testing.T) {
	Convey("a runner with 2% noise does not trip the proportional trail", t, func() {
		stop := NewStopLoss()
		stop.budget = NewBudget().Arm(0.005)

		_, _ = stop.Update(0.2)
		_, _ = stop.Update(20.0)
		_, ok := stop.Update(18.5)

		So(ok, ShouldBeFalse)
	})
}
