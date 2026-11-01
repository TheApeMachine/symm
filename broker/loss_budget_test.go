package broker

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestLossBudgetArmsFromFriction(t *testing.T) {
	Convey("wide friction widens invalidation under the hard ceiling", t, func() {
		budget := NewLossBudget().Arm(0.015)

		So(budget.armed, ShouldBeTrue)
		So(budget.maxLoss, ShouldBeGreaterThan, 2.0)
		So(budget.maxLoss, ShouldBeLessThanOrEqualTo, 4.0)
		So(budget.armPct, ShouldBeGreaterThanOrEqualTo, 1.5)
	})
}

func TestLossBudgetFreeze(t *testing.T) {
	Convey("re-arming cannot chase the floor wider after entry", t, func() {
		budget := NewLossBudget().Arm(0.005)
		first := budget.maxLoss

		budget.Arm(0.02)
		So(budget.maxLoss, ShouldEqual, first)
	})
}

func TestLossBudgetFallsBackToConfig(t *testing.T) {
	Convey("zero friction keeps the configured max loss", t, func() {
		budget := NewLossBudget().Arm(0)

		So(budget.maxLoss, ShouldAlmostEqual, 2.0, 0.001)
		So(budget.armPct, ShouldAlmostEqual, 0.5, 0.001)
	})
}
