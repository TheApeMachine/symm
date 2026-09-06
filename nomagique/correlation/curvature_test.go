package correlation

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestCurvatureNext(t *testing.T) {
	Convey("Given a Curvature as a Primitive", t, func() {
		curvature := NewCurvature(core.From(0.0))

		Convey("When I show it a profile peaking inside the range", func() {
			So(core.To[float64](curvature.Next(core.From(profileOf(2)))),
				ShouldAlmostEqual, 2*0.9-0.1-0.3, 1e-12)
		})

		Convey("When I show it several profiles in one step", func() {
			So(core.To[float64](curvature.Next(tests.NewRun(
				core.From(profileOf(2)),
				core.From([]Point{}),
			))), ShouldAlmostEqual, 2*0.9-0.1-0.3, 1e-12)
		})

		Convey("When the profile was sampled at no spacing", func() {
			So(core.To[float64](curvature.Next(core.From([]Point{
				{X: 0, Y: 0.1}, {X: 0, Y: 0.9}, {X: 0, Y: 0.2},
			}))), ShouldEqual, 0)
		})

		Convey("When the peak sits at the edge of the range", func() {
			curvature.Next(core.From(profileOf(2)))

			So(core.To[float64](curvature.Next(core.From(profileOf(4)))),
				ShouldAlmostEqual, 2*0.9-0.1-0.3, 1e-12)
		})

		Convey("When I offer nothing", func() {
			curvature.Next(core.From(profileOf(2)))

			So(core.To[float64](curvature.Next(nil)),
				ShouldAlmostEqual, 2*0.9-0.1-0.3, 1e-12)
		})
	})
}
