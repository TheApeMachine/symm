package broker

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestStructuralMilestoneSnap(t *testing.T) {
	Convey("milestone peaks snap the hard floor upward", t, func() {
		stop := NewStructuralStop()

		_, fired := stop.Update(0.0)
		So(fired, ShouldBeFalse)
		So(stop.Floor(), ShouldAlmostEqual, -2.0, 0.001)

		stop.Update(1.1)
		So(stop.Floor(), ShouldAlmostEqual, 0.35, 0.001)

		stop.Update(2.2)
		So(stop.Floor(), ShouldAlmostEqual, 1.0, 0.001)

		stop.Update(3.6)
		So(stop.Floor(), ShouldAlmostEqual, 2.2, 0.001)
	})
}

func TestStructuralFloorHit(t *testing.T) {
	Convey("dropping through a milestone floor ends the shadow", t, func() {
		stop := NewStructuralStop()
		stop.Update(0.0)
		stop.Update(1.2)

		trigger, fired := stop.Update(0.1)
		So(fired, ShouldBeTrue)
		So(trigger, ShouldContainSubstring, "structural floor")
		So(stop.Done(), ShouldBeTrue)
	})
}

func TestStructuralFailedRetest(t *testing.T) {
	Convey("a bounce that fails the prior peak then breaks mid fires exhaustion", t, func() {
		stop := NewStructuralStop()

		stop.Update(0.0)
		stop.Update(1.5)
		stop.Update(0.9)                   // noise pullback into trough
		stop.Update(1.1)                   // bounce starts, fails to reclaim 1.5
		stop.Update(1.2)                   // bounce high still below prior peak
		trigger, fired := stop.Update(1.0) // break below mid of 0.9..1.2

		So(fired, ShouldBeTrue)
		So(trigger, ShouldContainSubstring, "structural exhaustion")
	})
}

func TestStructuralHigherHighContinues(t *testing.T) {
	Convey("a healthy higher high does not fire exhaustion", t, func() {
		stop := NewStructuralStop()

		stop.Update(0.0)
		stop.Update(1.5)
		stop.Update(0.9)
		_, fired := stop.Update(1.7)

		So(fired, ShouldBeFalse)
		So(stop.Done(), ShouldBeFalse)
		So(stop.Peak(), ShouldAlmostEqual, 1.7, 0.001)
	})
}

func TestStructuralFrictionZone0(t *testing.T) {
	Convey("zone-0 respects a friction-widened loss budget", t, func() {
		stop := NewStructuralStop()
		stop.BindBudget(NewLossBudget().Arm(0.015))

		stop.Update(0.0)
		So(stop.Floor(), ShouldBeLessThan, -2.0)
		So(stop.Floor(), ShouldBeGreaterThanOrEqualTo, -4.0)

		_, fired := stop.Update(-2.5)
		So(fired, ShouldBeFalse)
	})
}
