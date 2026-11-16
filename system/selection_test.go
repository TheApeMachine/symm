package system

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestNewSelection(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When NewSelection is called", func() {
			sel := NewSelection()

			Convey("board caps match viper defaults", func() {
				So(sel.MaxOpenPositions, ShouldEqual, 5)
				So(sel.MaxEntersPerTick, ShouldEqual, 1)
				So(sel.MinBoardScore, ShouldEqual, 0.35)
				So(sel.BoardPeerWindow.Seconds(), ShouldEqual, 2)
				So(sel.MaxAccumPeakFrac, ShouldEqual, 0.85)
				So(sel.HeartbeatBatch, ShouldEqual, 48)
			})
		})
	})
}

func BenchmarkNewSelection(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		_ = NewSelection()
	}
}
