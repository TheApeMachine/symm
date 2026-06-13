package broker

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestDeriveTrailOffset(t *testing.T) {
	Convey("Given spread and tape volatility", t, func() {
		Convey("It should widen the trail as spread and vol rise", func() {
			tight := DeriveTrailOffset(10, 0.001)
			wide := DeriveTrailOffset(100, 0.01)
			So(wide, ShouldBeGreaterThan, tight)
		})

		Convey("It should respect the derived stop floor", func() {
			offset := DeriveTrailOffset(50, 0)
			floor := DeriveStopFloor(50, 0)
			So(offset, ShouldBeGreaterThanOrEqualTo, floor)
		})

		Convey("It should widen entry arm offset for bid below economic entry", func() {
			base := DeriveTrailOffset(10, 0)
			armed := DeriveEntryArmOffset(10, 100.30, 99.95)

			So(armed, ShouldBeGreaterThan, base)
		})
	})
}
