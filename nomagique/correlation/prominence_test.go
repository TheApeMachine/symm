package correlation

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestProminenceNext(t *testing.T) {
	Convey("Given a Prominence as a Primitive", t, func() {
		prominence := NewProminence(core.From(0.0))

		Convey("When I show it a profile peaking inside the range", func() {
			So(core.To[float64](prominence.Next(core.From(profileOf(2)))),
				ShouldAlmostEqual, 0.9-0.2, 1e-12)
		})

		Convey("When I show it several profiles in one step", func() {
			So(core.To[float64](prominence.Next(tests.NewRun(
				core.From(profileOf(2)),
				core.From([]Point{}),
			))), ShouldAlmostEqual, 0.9-0.2, 1e-12)
		})

		Convey("When the peak sits at the edge of the range", func() {
			prominence.Next(core.From(profileOf(2)))

			So(core.To[float64](prominence.Next(core.From(profileOf(0)))),
				ShouldAlmostEqual, 0.9-0.2, 1e-12)
		})

		Convey("When I offer nothing", func() {
			prominence.Next(core.From(profileOf(2)))

			So(core.To[float64](prominence.Next(nil)),
				ShouldAlmostEqual, 0.9-0.2, 1e-12)
		})
	})
}
