package calculus

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/temporal"
	"github.com/theapemachine/symm/nomagique/types"
)

func TestDecay(t *testing.T) {
	Convey("Given a clock halfway through its span", t, func() {
		decay := NewDecay(temporal.NewClock(1, 2), nil)
		level := types.NewValue(10.0)

		Convey("Write then Read should walk linearly toward zero", func() {
			So(decay.Write(level), ShouldBeNil)
			So(decay.Read(level), ShouldBeNil)
			So(level.Value(), ShouldEqual, 5)
		})

		Convey("Read before Write should fail", func() {
			fresh := NewDecay(temporal.NewClock(1, 2), nil)
			So(fresh.Read(types.NewInput[float64]()), ShouldNotBeNil)
		})

		Convey("An exponential shape should apply one e-folding per span", func() {
			shaped := NewDecay(temporal.NewClock(1, 2), NewExponential())
			So(shaped.Write(level), ShouldBeNil)
			So(shaped.Read(level), ShouldBeNil)
			So(level.Value(), ShouldAlmostEqual, 10*math.Exp(-0.5), 1e-12)
		})

		Convey("Reset should require a new write", func() {
			So(decay.Write(types.NewValue(10.0)), ShouldBeNil)
			So(decay.Reset(), ShouldBeNil)
			So(decay.Read(types.NewInput[float64]()), ShouldNotBeNil)
		})

		Convey("Close should succeed", func() {
			So(decay.Close(), ShouldBeNil)
		})
	})
}
