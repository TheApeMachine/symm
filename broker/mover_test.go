package broker

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestMoverApplyVolume(t *testing.T) {
	Convey("mover stores ticker quote volume and change", t, func() {
		mover := NewMover()
		mover.Apply("STRK/USD", 120_000, 22.5)

		So(mover.VolumeUSD("STRK/USD"), ShouldEqual, 120_000)
		So(mover.ChangePct("STRK/USD"), ShouldEqual, 22.5)
		So(mover.VolumeUSD("QUIET/USD"), ShouldEqual, 0)
	})
}
