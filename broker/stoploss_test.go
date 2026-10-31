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
