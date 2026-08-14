package calculus

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/temporal"
	"github.com/theapemachine/symm/nomagique/types"
)

func TestAttack(t *testing.T) {
	Convey("Given a clock halfway through its span", t, func() {
		attack := NewAttack(temporal.NewClock(1, 2), nil)
		peak := types.NewValue(10.0)

		Convey("Write then Read should walk linearly toward the peak", func() {
			So(attack.Write(peak), ShouldBeNil)
			So(attack.Read(peak), ShouldBeNil)
			So(peak.Value(), ShouldEqual, 5)
		})

		Convey("Read before Write should fail", func() {
			fresh := NewAttack(temporal.NewClock(1, 2), nil)
			So(fresh.Read(types.NewInput[float64]()), ShouldNotBeNil)
		})

		Convey("An exponential shape should rise as one minus remaining decay", func() {
			shaped := NewAttack(temporal.NewClock(1, 2), NewExponential())
			So(shaped.Write(types.NewValue(10.0)), ShouldBeNil)
			So(shaped.Read(peak), ShouldBeNil)
			So(peak.Value(), ShouldAlmostEqual, 10*(1-math.Exp(-0.5)), 1e-12)
		})

		Convey("Reset should require a new write", func() {
			So(attack.Write(types.NewValue(10.0)), ShouldBeNil)
			So(attack.Reset(), ShouldBeNil)
			So(attack.Read(types.NewInput[float64]()), ShouldNotBeNil)
		})

		Convey("Close should succeed", func() {
			So(attack.Close(), ShouldBeNil)
		})
	})
}
