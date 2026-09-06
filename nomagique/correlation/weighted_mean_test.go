package correlation

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestWeightedMeanNext(t *testing.T) {
	Convey("Given a WeightedMean as a Primitive", t, func() {
		mean := NewWeightedMean(core.From(0.0))

		Convey("When I show it a weighted cross-section", func() {
			So(core.To[float64](mean.Next(core.From([]Point{
				{X: 3, Y: 1}, {X: 1, Y: -1},
			}))), ShouldAlmostEqual, 0.5, 1e-12)
		})

		Convey("When I show it several cross-sections in one step", func() {
			So(core.To[float64](mean.Next(tests.NewRun(
				core.From([]Point{{X: 3, Y: 1}}),
				core.From([]Point{{X: 1, Y: -1}}),
			))), ShouldAlmostEqual, 0.5, 1e-12)
		})

		Convey("When a value is poisoned", func() {
			So(math.IsNaN(core.To[float64](mean.Next(core.From([]Point{
				{X: 1, Y: math.NaN()}, {X: 1, Y: 1},
			})))), ShouldBeTrue)
		})

		Convey("When nothing carries any weight", func() {
			mean.Next(core.From([]Point{{X: 2, Y: 4}}))

			So(core.To[float64](mean.Next(core.From([]Point{{X: 0, Y: 9}}))),
				ShouldEqual, 4)
		})

		Convey("When I offer nothing", func() {
			mean.Next(core.From([]Point{{X: 2, Y: 4}}))

			So(core.To[float64](mean.Next(nil)), ShouldEqual, 4)
		})
	})
}
