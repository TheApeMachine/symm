package correlation

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/tests"
)

/*
returnsOf is the returns of the shared observation run, which the estimates
downstream of a path are all driven by.
*/
func returnsOf(count int) []Interval {
	return intervalsOf(observations(count))
}

func TestEnergiesNext(t *testing.T) {
	Convey("Given Energies as a Primitive", t, func() {
		energies := NewEnergies(core.From([]float64(nil)))

		Convey("When I show it one run", func() {
			rates := core.To[[]float64](
				energies.Next(core.From(returnsOf(4))),
			)

			So(len(rates), ShouldEqual, 3)
			So(rates[0], ShouldAlmostEqual, math.Ln2*math.Ln2, 1e-12)
		})

		Convey("When I show it several runs in one step", func() {
			rates := core.To[[]float64](energies.Next(tests.NewRun(
				core.From(returnsOf(3)),
				core.From(returnsOf(4)),
			)))

			So(len(rates), ShouldEqual, 5)
		})

		Convey("When an interval spans no time", func() {
			rates := core.To[[]float64](energies.Next(core.From(
				[]Interval{{From: 4, To: 4, Value: 1}},
			)))

			So(len(rates), ShouldEqual, 0)
		})

		Convey("When a return is poisoned", func() {
			rates := core.To[[]float64](energies.Next(core.From(
				[]Interval{{From: 0, To: 1, Value: math.NaN()}},
			)))

			So(math.IsNaN(rates[0]), ShouldBeTrue)
		})

		Convey("When I offer nothing", func() {
			So(len(core.To[[]float64](energies.Next(nil))), ShouldEqual, 0)
		})
	})
}
