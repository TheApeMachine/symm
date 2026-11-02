package stoploss

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestStructuralResumeKeepsFloorDropsPhase(t *testing.T) {
	Convey("Resume keeps locked peak/floor and reseeds phase from peak", t, func() {
		stop := NewStructural()
		stop.Arm(0.01)

		_, _ = stop.Update(0.1)
		_, _ = stop.Update(2.5)
		_, ok := stop.Update(2.0)
		So(ok, ShouldBeFalse)

		machine := stop.Machine()
		So(machine.Initialized, ShouldBeTrue)
		So(machine.PeakPnL, ShouldBeGreaterThan, 0)

		restored := NewStructural()
		restored.Resume(machine)

		So(restored.Machine().Floor, ShouldEqual, machine.Floor)
		So(restored.Machine().PeakPnL, ShouldEqual, machine.PeakPnL)
		So(restored.Machine().Milestone, ShouldEqual, machine.Milestone)
		So(restored.phase, ShouldEqual, structuralPhasePeak)

		raw, err := restored.MarshalJSON()
		So(err, ShouldBeNil)

		round := NewStructural()
		So(round.UnmarshalJSON(raw), ShouldBeNil)
		So(round.Machine().Floor, ShouldEqual, machine.Floor)
		So(round.phase, ShouldEqual, structuralPhasePeak)
	})
}

func TestStructuralColdInitAnchorsAtZero(t *testing.T) {
	Convey("first Update after a gap anchors entry at 0 not current mark", t, func() {
		stop := NewStructural()
		stop.Arm(0.01)

		_, _ = stop.Update(3.0)
		So(stop.entryPnL, ShouldEqual, 0)
		So(stop.peakPnL, ShouldEqual, 3.0)
		So(stop.phase, ShouldEqual, structuralPhaseIdle)
		So(stop.floor, ShouldBeGreaterThanOrEqualTo, -stop.budget.maxLoss)
	})
}

func TestStructuralArmIgnoresZeroFriction(t *testing.T) {
	Convey("Arm(0) does not freeze; later friction widens the floor", t, func() {
		stop := NewStructural()
		provisional := stop.budget.maxLoss

		stop.Arm(0)
		So(stop.armed, ShouldBeFalse)

		_, _ = stop.Update(-1.5)
		So(stop.floor, ShouldEqual, -provisional)

		stop.Arm(0.02) // 2% friction → wider invalidation via mult
		So(stop.armed, ShouldBeTrue)
		So(stop.budget.maxLoss, ShouldBeGreaterThan, provisional)
		So(stop.floor, ShouldEqual, -stop.budget.maxLoss)
	})
}

func TestStructuralIgnoresUnderwaterRetest(t *testing.T) {
	Convey("spread wobble under water does not fire exhaustion or ratchet floor", t, func() {
		stop := NewStructural()
		stop.Arm(0.01)

		_, _ = stop.Update(-1.19)
		floor := stop.floor

		_, ok := stop.Update(-1.64)
		So(ok, ShouldBeFalse)
		_, ok = stop.Update(-1.10)
		So(ok, ShouldBeFalse)
		_, ok = stop.Update(-1.97)
		So(ok, ShouldBeFalse)

		So(stop.phase, ShouldEqual, structuralPhaseIdle)
		So(stop.floor, ShouldEqual, floor)
		So(stop.done, ShouldBeFalse)
	})
}

func TestStructuralExhaustionOnlyInProfit(t *testing.T) {
	Convey("failed retest fires after clearing arm and rolling over mid bounce", t, func() {
		stop := NewStructural()
		stop.Arm(0.005)

		_, _ = stop.Update(0.1)
		_, _ = stop.Update(6.0) // clears arm + enters peak
		_, _ = stop.Update(4.0) // pullback ≥ 1.0% noise
		_, _ = stop.Update(4.8) // bounce misses prior peak by ≥ noise
		trigger, ok := stop.Update(4.2)

		So(ok, ShouldBeTrue)
		So(trigger, ShouldContainSubstring, "structural exhaustion")
	})
}
