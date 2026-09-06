package correlation

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestEnergyNext(t *testing.T) {
	Convey("Given Energy as a Primitive", t, func() {
		energy := NewEnergy(core.From(0.0))

		Convey("When I show it one run", func() {
			So(core.To[float64](energy.Next(core.From(returnsOf(4)))),
				ShouldAlmostEqual, 3*math.Ln2*math.Ln2, 1e-12)
		})

		Convey("When I show it several runs in one step", func() {
			So(core.To[float64](energy.Next(tests.NewRun(
				core.From(returnsOf(2)),
				core.From(returnsOf(2)),
			))), ShouldAlmostEqual, 2*math.Ln2*math.Ln2, 1e-12)
		})

		Convey("When I show it a second run", func() {
			energy.Next(core.From(returnsOf(4)))

			So(core.To[float64](energy.Next(core.From(returnsOf(2)))),
				ShouldAlmostEqual, math.Ln2*math.Ln2, 1e-12)
		})

		Convey("When a return is poisoned", func() {
			So(math.IsNaN(core.To[float64](energy.Next(core.From(
				[]Interval{{From: 0, To: 1, Value: math.Inf(1)}},
			)))), ShouldBeFalse)
			So(math.IsInf(core.To[float64](energy.Next(core.From(
				[]Interval{{From: 0, To: 1, Value: math.Inf(1)}},
			))), 1), ShouldBeTrue)
		})

		Convey("When I offer nothing", func() {
			So(core.To[float64](energy.Next(nil)), ShouldEqual, 0)
		})
	})
}
