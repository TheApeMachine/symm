package adaptive

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestIlliquidityScore(t *testing.T) {
	Convey("Given quote volume below the peer median", t, func() {
		score := IlliquidityScore(100, []float64{200, 300, 400})

		Convey("It should return a unitless score in (0, 1]", func() {
			So(score, ShouldBeGreaterThan, 0)
			So(score, ShouldBeLessThanOrEqualTo, 1)
		})
	})

	Convey("Given quote volume at or above the peer median", t, func() {
		Convey("It should return zero", func() {
			So(IlliquidityScore(300, []float64{200, 300, 400}), ShouldEqual, 0)
			So(IlliquidityScore(500, []float64{200, 300, 400}), ShouldEqual, 0)
		})
	})
}
