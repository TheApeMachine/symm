package broker

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestLossBudgetArmsFromFriction(t *testing.T) {
	Convey("wide friction widens invalidation under the hard ceiling", t, func() {
		budget := NewLossBudget().Arm(0.015) // 1.5% of mid

		So(budget.Armed(), ShouldBeTrue)
		So(budget.MaxLoss(), ShouldBeGreaterThan, 2.0)
		So(budget.MaxLoss(), ShouldBeLessThanOrEqualTo, 4.0)
		So(budget.ArmPct(), ShouldBeGreaterThanOrEqualTo, 1.5)
	})
}

func TestLossBudgetFreeze(t *testing.T) {
	Convey("re-arming cannot chase the floor wider after entry", t, func() {
		budget := NewLossBudget().Arm(0.005)
		first := budget.MaxLoss()

		budget.Arm(0.02)
		So(budget.MaxLoss(), ShouldEqual, first)
	})
}

func TestLossBudgetFallsBackToConfig(t *testing.T) {
	Convey("zero friction keeps the configured max loss", t, func() {
		budget := NewLossBudget().Arm(0)

		So(budget.MaxLoss(), ShouldAlmostEqual, 2.0, 0.001)
		So(budget.ArmPct(), ShouldAlmostEqual, 0.5, 0.001)
	})
}
