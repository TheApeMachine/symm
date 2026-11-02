package stoploss

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestRailsLearnWidensTrailOnShadowWin(t *testing.T) {
	Convey("a better green shadow exit widens giveback and lowers arm", t, func() {
		rails := NewRails()
		before := rails.Snapshot()

		rails.Learn(0.5, 1.5) // shadow beat official by +1.0 in profit

		after := rails.Snapshot()
		So(rails.Episodes(), ShouldEqual, 1)
		So(rails.Edge(), ShouldBeGreaterThan, 0)
		So(after.BaseGiveback, ShouldBeGreaterThan, before.BaseGiveback)
		So(after.ArmPct, ShouldBeLessThanOrEqualTo, before.ArmPct)
		So(after.MaxLoss, ShouldEqual, before.MaxLoss)
	})
}

func TestRailsLearnTightensTrailOnShadowLoss(t *testing.T) {
	Convey("a worse green shadow exit tightens giveback", t, func() {
		rails := NewRails()
		before := rails.Snapshot()

		rails.Learn(1.5, 0.2) // shadow shook out early while green

		after := rails.Snapshot()
		So(after.BaseGiveback, ShouldBeLessThan, before.BaseGiveback)
		So(after.MaxLoss, ShouldEqual, before.MaxLoss)
	})
}

func TestRailsLearnWidensInvalidationOnRedShadowLoss(t *testing.T) {
	Convey("a worse red shadow exit widens maxLoss only", t, func() {
		rails := NewRails()
		before := rails.Snapshot()

		rails.Learn(-1.0, -2.5)

		after := rails.Snapshot()
		So(after.MaxLoss, ShouldBeGreaterThan, before.MaxLoss)
		So(after.BaseGiveback, ShouldEqual, before.BaseGiveback)
	})
}

func TestRailsLearnTightensInvalidationOnRedShadowWin(t *testing.T) {
	Convey("a better red shadow exit tightens maxLoss only", t, func() {
		rails := NewRails()
		before := rails.Snapshot()

		rails.Learn(-2.5, -1.0)

		after := rails.Snapshot()
		So(after.MaxLoss, ShouldBeLessThan, before.MaxLoss)
		So(after.BaseGiveback, ShouldEqual, before.BaseGiveback)
	})
}

func TestRailsSnapshotFreeze(t *testing.T) {
	Convey("snapshots do not chase later Learn updates", t, func() {
		rails := NewRails()
		snap := rails.Snapshot()

		rails.Learn(0.0, 2.0)
		So(rails.Snapshot().BaseGiveback, ShouldNotEqual, snap.BaseGiveback)
		So(snap.BaseGiveback, ShouldEqual, NewRails().Snapshot().BaseGiveback)
	})
}

func TestRailsBlobRoundTripFields(t *testing.T) {
	Convey("Learn mutates the blob fields that AttachStore would restore", t, func() {
		rails := NewRails()
		before := rails.Snapshot()
		rails.Learn(0.5, 1.5)

		So(rails.Episodes(), ShouldEqual, 1)
		So(rails.Snapshot().BaseGiveback, ShouldNotEqual, before.BaseGiveback)
		So(rails.Edge(), ShouldNotEqual, 0)
	})
}
