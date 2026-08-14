package nomagique

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/temporal"
	"github.com/theapemachine/symm/nomagique/types"
)

func TestNumber(t *testing.T) {
	Convey("Given a level and a linear decay", t, func() {
		level := types.NewValue(10.0)
		decay := calculus.NewDecay(temporal.NewClock(1, 2), nil)

		Convey("It should FlipFlop write then read onto the datapoint", func() {
			So(Number(level, decay), ShouldBeNil)
			So(level.Value(), ShouldEqual, 5)
		})
	})

	Convey("Given a level, a clock, and an exponential shape", t, func() {
		level := types.NewValue(10.0)
		decay := calculus.NewDecay(temporal.NewClock(1, 2), calculus.NewExponential())

		Convey("It should compose decay, shape, and timing", func() {
			So(Number(level, decay), ShouldBeNil)
			So(level.Value(), ShouldAlmostEqual, 10*math.Exp(-0.5), 1e-12)
		})
	})
}
