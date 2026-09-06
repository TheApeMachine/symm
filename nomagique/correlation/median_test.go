package correlation

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestMedianNext(t *testing.T) {
	Convey("Given a Median as a Primitive", t, func() {
		median := NewMedian(core.From(0.0))

		Convey("When I show it an odd run", func() {
			So(core.To[float64](median.Next(core.From([]float64{9, 1, 5}))),
				ShouldEqual, 5)
		})

		Convey("When I show it an even run", func() {
			So(core.To[float64](median.Next(core.From([]float64{1, 3, 5, 9}))),
				ShouldEqual, 4)
		})

		Convey("When I show it several runs in one step", func() {
			So(core.To[float64](median.Next(tests.NewRun(
				core.From([]float64{1, 3}),
				core.From([]float64{5}),
			))), ShouldEqual, 3)
		})

		Convey("When a value is poisoned", func() {
			So(math.IsNaN(core.To[float64](
				median.Next(core.From([]float64{1, math.NaN(), 3})),
			)), ShouldBeTrue)
		})

		Convey("When I show it an empty run", func() {
			median.Next(core.From([]float64{7}))

			So(core.To[float64](median.Next(core.From([]float64{}))), ShouldEqual, 7)
		})

		Convey("When I offer nothing", func() {
			median.Next(core.From([]float64{7}))

			So(core.To[float64](median.Next(nil)), ShouldEqual, 7)
		})
	})
}
