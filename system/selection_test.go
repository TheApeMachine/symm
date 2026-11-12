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
		So(sel.StoplossFloorMinPnlPct, ShouldEqual, -1)
	})
}
