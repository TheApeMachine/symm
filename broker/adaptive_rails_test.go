package broker

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestAdaptiveRailsLearnTightensOnShadowWin(t *testing.T) {
	Convey("a better shadow exit tightens giveback and arm", t, func() {
		rails := NewAdaptiveRails()
		before := rails.Snapshot()

		rails.Learn(0.5, 1.5) // shadow beat official by +1.0

		after := rails.Snapshot()
		So(rails.Episodes(), ShouldEqual, 1)
		So(rails.Edge(), ShouldBeGreaterThan, 0)
		So(after.BaseGiveback, ShouldBeLessThan, before.BaseGiveback)
		So(after.ArmPct, ShouldBeLessThanOrEqualTo, before.ArmPct)
	})
}

func TestAdaptiveRailsLearnWidensOnShadowLoss(t *testing.T) {
	Convey("a worse shadow exit widens the rails", t, func() {
		rails := NewAdaptiveRails()
		before := rails.Snapshot()

		rails.Learn(1.5, 0.2) // shadow shook out early

		after := rails.Snapshot()
		So(after.BaseGiveback, ShouldBeGreaterThan, before.BaseGiveback)
		So(after.MaxLoss, ShouldBeGreaterThanOrEqualTo, before.MaxLoss)
	})
}

func TestAdaptiveRailsSnapshotFreeze(t *testing.T) {
	Convey("snapshots do not chase later Learn updates", t, func() {
		rails := NewAdaptiveRails()
		snap := rails.Snapshot()

		rails.Learn(0.0, 2.0)
		So(rails.Snapshot().BaseGiveback, ShouldNotEqual, snap.BaseGiveback)
		So(snap.BaseGiveback, ShouldEqual, NewAdaptiveRails().Snapshot().BaseGiveback)
	})
}
