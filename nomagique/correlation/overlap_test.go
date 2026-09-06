package correlation

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestOverlapNext(t *testing.T) {
	Convey("Given an Overlap as a Primitive", t, func() {
		counter := core.From(returnsOf(4))

		Convey("When the two runs coincide", func() {
			overlap := NewOverlap(counter, core.From(0.0))

			paired := core.To[[]float64](overlap.Next(core.From(returnsOf(4))))

			So(len(paired), ShouldEqual, 3)
			So(paired[0], ShouldAlmostEqual, math.Ln2*math.Ln2, 1e-12)
		})

		Convey("When the incoming run is shifted clear of the counter run", func() {
			overlap := NewOverlap(counter, core.From(100*NanosPerSecond))

			So(len(core.To[[]float64](
				overlap.Next(core.From(returnsOf(4))),
			)), ShouldEqual, 0)
		})

		Convey("When the incoming run is shifted by one sampling interval", func() {
			overlap := NewOverlap(counter, core.From(1*NanosPerSecond))

			So(len(core.To[[]float64](
				overlap.Next(core.From(returnsOf(4))),
			)), ShouldEqual, 2)
		})

		Convey("When I show it several runs in one step", func() {
			overlap := NewOverlap(counter, core.From(0.0))

			So(len(core.To[[]float64](overlap.Next(tests.NewRun(
				core.From(returnsOf(2)),
				core.From(returnsOf(2)),
			)))), ShouldEqual, 2)
		})

		Convey("When the counter run holds nothing", func() {
			overlap := NewOverlap(core.From([]Interval(nil)), core.From(0.0))

			So(len(core.To[[]float64](
				overlap.Next(core.From(returnsOf(4))),
			)), ShouldEqual, 0)
		})

		Convey("When a return is poisoned", func() {
			overlap := NewOverlap(
				core.From([]Interval{{From: 0, To: 2, Value: math.NaN()}}),
				core.From(0.0),
			)

			paired := core.To[[]float64](overlap.Next(core.From(
				[]Interval{{From: 0, To: 2, Value: 1}},
			)))

			So(math.IsNaN(paired[0]), ShouldBeTrue)
		})

		Convey("When I offer nothing", func() {
			overlap := NewOverlap(counter, core.From(0.0))

			So(len(core.To[[]float64](overlap.Next(nil))), ShouldEqual, 0)
		})
	})
}
