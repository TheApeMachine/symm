package broker

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

// The YALA/EUR incident: float stepping computed 20000/0.00001 as
// 1999999999.9999998, floored one full increment short, and the desk submitted
// an entry that could never be sold. The grid is exact decimal arithmetic now.
func TestGridArithmeticIsExact(t *testing.T) {
	Convey("Given the YALA-scale grid (increment 0.00001)", t, func() {
		Convey("A venue-minimum quantity stays on its own grid point", func() {
			So(orderStepsDown(20000, 0.00001), ShouldEqual, 2e9)
			So(roundDownToIncrement(20000, 0.00001), ShouldEqual, 20000)
			So(roundUpToIncrement(20000, 0.00001), ShouldEqual, 20000)
		})

		Convey("On-grid values are aligned by identity, off-grid are not", func() {
			So(isAligned(20000, 0.00001), ShouldBeTrue)
			So(isAligned(19999.99999, 0.00001), ShouldBeTrue) // one step below — on grid
			So(isAligned(20000.000003, 0.00001), ShouldBeFalse)
		})

		Convey("Flooring an off-grid value lands exactly one step down", func() {
			So(roundDownToIncrement(20000.000003, 0.00001), ShouldEqual, 20000)
			So(roundUpToIncrement(20000.000003, 0.00001), ShouldEqual, 20000.00001)
		})
	})

	Convey("Given coarse grids", t, func() {
		So(roundDownToIncrement(19999.99999, 1), ShouldEqual, 19999)
		So(roundUpToIncrement(19999.99999, 1), ShouldEqual, 20000)
		So(isAligned(7, 0.5), ShouldBeTrue)
		So(isAligned(7.3, 0.5), ShouldBeFalse)
	})
}
