package correlation

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestEffectiveCountNext(t *testing.T) {
	Convey("Given an EffectiveCount as a Primitive", t, func() {
		count := NewEffectiveCount(core.From(0.0))

		Convey("When every point carries the same weight", func() {
			So(core.To[float64](count.Next(core.From([]Point{
				{X: 2, Y: 0}, {X: 2, Y: 0}, {X: 2, Y: 0}, {X: 2, Y: 0},
			}))), ShouldAlmostEqual, 4, 1e-12)
		})

		Convey("When one point dominates the cross-section", func() {
			So(core.To[float64](count.Next(core.From([]Point{
				{X: 1000, Y: 0}, {X: 1, Y: 0}, {X: 1, Y: 0},
			}))), ShouldBeLessThan, 1.01)
		})

		Convey("When I show it several cross-sections in one step", func() {
			So(core.To[float64](count.Next(tests.NewRun(
				core.From([]Point{{X: 2, Y: 0}, {X: 2, Y: 0}}),
				core.From([]Point{{X: 2, Y: 0}, {X: 2, Y: 0}}),
			))), ShouldAlmostEqual, 4, 1e-12)
		})

		Convey("When nothing carries any weight", func() {
			count.Next(core.From([]Point{{X: 2, Y: 0}}))

			So(core.To[float64](count.Next(core.From([]Point{{X: 0, Y: 0}}))),
				ShouldEqual, 1)
		})

		Convey("When I offer nothing", func() {
			count.Next(core.From([]Point{{X: 2, Y: 0}}))

			So(core.To[float64](count.Next(nil)), ShouldEqual, 1)
		})
	})
}
