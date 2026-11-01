package broker

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestStopLossInvalidation(t *testing.T) {
	Convey("a 2pp adverse drop from the entry mark fires invalidation", t, func() {
		stop := NewStopLoss()

		_, armed := stop.Update(-1.0)
		So(armed, ShouldBeFalse)

		trigger, fired := stop.Update(-3.0)
		So(fired, ShouldBeTrue)
		So(trigger, ShouldContainSubstring, "invalidation stop")
	})
}

func TestStopLossTrailing(t *testing.T) {
	Convey("giveback past the allowed trail fires take-profit once green", t, func() {
		stop := NewStopLoss()

		stop.Update(0.0)
		stop.Update(3.0)

		state := stop.State(3.0)
		So(state.PeakPct, ShouldEqual, 3.0)
		So(state.Trailing, ShouldEqual, 0)

		trigger, fired := stop.Update(1.0)
		So(fired, ShouldBeTrue)
		So(trigger, ShouldContainSubstring, "trailing take-profit")
	})
}

func TestStopStateDamageTightensTrail(t *testing.T) {
	Convey("damage from a deep drawdown shrinks allowed giveback", t, func() {
		clean := NewStopLoss()
		clean.Update(0.0)
		clean.Update(4.0)
		cleanState := clean.State(4.0)

		hurt := NewStopLoss()
		hurt.Update(0.0)
		hurt.Update(-1.5)
		hurt.Update(4.0)
		hurtState := hurt.State(4.0)

		So(hurtState.Damage, ShouldBeGreaterThan, 0)
		So(hurtState.AllowedGivebackPct, ShouldBeLessThan, cleanState.AllowedGivebackPct)
	})
}

func TestStopLossActiveFloor(t *testing.T) {
	Convey("active floor uses trail only while green; red falls back to invalidation", t, func() {
		stop := NewStopLoss()
		stop.Update(0.0)

		So(stop.ActiveFloor(0.0), ShouldAlmostEqual, -2.0, 0.001)
		So(stop.State(0.0).FloorPct, ShouldAlmostEqual, -2.0, 0.001)
		So(stop.State(0.0).MaxLossPct, ShouldAlmostEqual, 2.0, 0.001)

		stop.Update(3.0)
		green := stop.ActiveFloor(3.0)
		So(green, ShouldBeGreaterThan, -2.0)
		So(green, ShouldAlmostEqual, 3.0-stop.State(3.0).AllowedGivebackPct, 0.001)

		So(stop.ActiveFloor(-0.5), ShouldAlmostEqual, -2.0, 0.001)
		So(stop.Proximity(3.0), ShouldBeLessThan, 1.0)
		So(stop.Proximity(-2.0), ShouldAlmostEqual, 1.0, 0.001)
	})
}

func TestStopLossFrictionBudget(t *testing.T) {
	Convey("a friction-armed budget delays invalidation past the static 2pp cut", t, func() {
		stop := NewStopLoss()
		stop.BindBudget(NewLossBudget().Arm(0.015))

		_, armed := stop.Update(0.0)
		So(armed, ShouldBeFalse)

		_, early := stop.Update(-2.5)
		So(early, ShouldBeFalse)

		trigger, fired := stop.Update(-3.5)
		So(fired, ShouldBeTrue)
		So(trigger, ShouldContainSubstring, "invalidation stop")
	})
}
