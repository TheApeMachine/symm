package system_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/system"
)

func TestNewSelection(t *testing.T) {
	Convey("Selection defaults include board caps", t, func() {
		sel := system.NewSelection()

		So(sel.MaxOpenPositions, ShouldEqual, 5)
		So(sel.MaxEntersPerTick, ShouldEqual, 1)
		So(sel.MaxAccumPeakFrac, ShouldEqual, 0.35)
		So(sel.StoplossArmPnlPct, ShouldEqual, 1.0)
		So(sel.StoplossRateDecayMin, ShouldEqual, 0.7)
		So(sel.StoplossPumpConfirmMin, ShouldEqual, 0.35)
		So(sel.StoplossConfirmMarks, ShouldEqual, 3)
		So(sel.StoplossNearPeakFrac, ShouldEqual, 0.98)
	})
}
