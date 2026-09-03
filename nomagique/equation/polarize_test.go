package equation

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/types"
)

func TestPolarize(t *testing.T) {
	Convey("Given a Polarize equation", t, func() {
		p := &Polarize{}

		Convey("A positive value populates Alpha and zeroes Beta", func() {
			out := p.StepScaled(types.Scalar(10.0), types.Scalar(10.0))

			So(float64(p.Alpha()), ShouldEqual, 10.0)
			So(float64(p.Beta()), ShouldEqual, 0.0)
			So(float64(p.AlphaNormalized()), ShouldEqual, 0.5)
			So(float64(p.BetaNormalized()), ShouldEqual, 0.0)
			So(float64(out), ShouldEqual, 0.5)
		})

		Convey("A negative value populates Beta and zeroes Alpha", func() {
			out := p.StepScaled(types.Scalar(-10.0), types.Scalar(10.0))

			So(float64(p.Alpha()), ShouldEqual, 0.0)
			So(float64(p.Beta()), ShouldEqual, 10.0)
			So(float64(p.AlphaNormalized()), ShouldEqual, 0.0)
			So(float64(p.BetaNormalized()), ShouldEqual, 0.5)
			So(float64(out), ShouldEqual, -0.5)
		})
	})
}
