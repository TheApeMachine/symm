package equation

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestLocalRegression(t *testing.T) {
	Convey("Given a LocalRegression equation", t, func() {
		reg := &LocalRegression{}

		Convey("with fewer than 2 prior samples, no slope is fit", func() {
			reg.Step(1.0, 1_000_000_000, 10.0)
			_, hasSlope := reg.Slope()
			So(hasSlope, ShouldBeFalse)
		})

		Convey("with linear samples, slope matches the line", func() {
			reg.Step(10.0, 1_000_000_000, 10.0)
			reg.Step(20.0, 2_000_000_000, 10.0)
			reg.Step(30.0, 3_000_000_000, 10.0)

			slope, hasSlope := reg.Slope()
			So(hasSlope, ShouldBeTrue)
			So(slope, ShouldAlmostEqual, 10.0, 1e-6)
		})
	})
}
