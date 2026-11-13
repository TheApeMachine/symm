package system

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestNewSelection(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When NewSelection is called", func() {
			sel := NewSelection()

			Convey("board caps and stoploss knobs match viper defaults", func() {
				So(sel.MaxOpenPositions, ShouldEqual, 5)
				So(sel.MaxEntersPerTick, ShouldEqual, 1)
				So(sel.MaxAccumPeakFrac, ShouldEqual, 0.35)
				So(sel.StoplossArmPnlPct, ShouldEqual, 1.0)
				So(sel.StoplossRateDecayMin, ShouldEqual, 0.7)
				So(sel.StoplossPumpConfirmMin, ShouldEqual, 0.35)
				So(sel.StoplossConfirmMarks, ShouldEqual, 3)
				So(sel.StoplossNearPeakFrac, ShouldEqual, 0.98)
				So(sel.HeartbeatBatch, ShouldEqual, 48)
			})
		})
	})
}
