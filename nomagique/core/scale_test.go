package core

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestScaleRules(t *testing.T) {
	Convey("Given the shared definedness rules", t, func() {
		Convey("the minimum prior count is derived from the tolerance", func() {
			So(MinimumPrior, ShouldEqual, 9)
			// At n = 9 the sigma's relative standard error is exactly 1/4.
			So(1/math.Sqrt(2*(MinimumPrior-1)), ShouldBeLessThanOrEqualTo, Tolerance)
			So(1/math.Sqrt(2*(MinimumPrior-2)), ShouldBeGreaterThan, Tolerance)
		})

		Convey("a scale is negligible only relative to its magnitudes", func() {
			So(Negligible(8e-18, 3.6, 0.07), ShouldBeTrue)
			So(Negligible(1e-9, 1e-6), ShouldBeFalse)
			So(Negligible(0, 0), ShouldBeTrue)
		})

		Convey("a prior scale needs enough samples, dispersion, and resolution", func() {
			_, ok := PriorScale(MinimumPrior-1, 8, 1, 0)
			So(ok, ShouldBeFalse)

			scale, ok := PriorScale(MinimumPrior, 8, 1, 0)
			So(ok, ShouldBeTrue)
			So(scale, ShouldEqual, 1)

			_, ok = PriorScale(MinimumPrior, 0, 1, 0)
			So(ok, ShouldBeFalse)

			_, ok = PriorScale(MinimumPrior, 1e-34, 3.6, 0.07)
			So(ok, ShouldBeFalse)
		})

		Convey("an interval is resolvable once the clock grain is within tolerance of it", func() {
			grain := ClockResolution.Seconds()
			So(Resolvable(0), ShouldBeFalse)
			So(Resolvable(grain), ShouldBeFalse)
			So(Resolvable(3*grain), ShouldBeFalse)
			So(Resolvable(4*grain), ShouldBeTrue)
			So(Resolvable(1), ShouldBeTrue)
		})
	})
}
