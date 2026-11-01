package stoploss

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestStructuralResumeKeepsFloorDropsPhase(t *testing.T) {
	Convey("Resume keeps locked peak/floor and reseeds phase from peak", t, func() {
		stop := NewStructural()
		stop.Arm(0)

		_, _ = stop.Update(0.1)
		_, _ = stop.Update(2.5)
		_, ok := stop.Update(2.0) // pullback noise, no exit
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
		stop.Arm(0)

		_, _ = stop.Update(3.0)
		So(stop.entryPnL, ShouldEqual, 0)
		So(stop.peakPnL, ShouldEqual, 3.0)
		So(stop.floor, ShouldBeGreaterThanOrEqualTo, -stop.budget.maxLoss)
	})
}
