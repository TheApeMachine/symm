package calculus

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/types"
)

func TestExponential(t *testing.T) {
	Convey("Given an exponential shape", t, func() {
		shape := NewExponential()

		Convey("Unit progress should leave one e-folding", func() {
			remaining := types.NewInput[float64]()
			So(shape.Write(types.NewValue(1.0)), ShouldBeNil)
			So(shape.Read(remaining), ShouldBeNil)
			So(remaining.Value(), ShouldAlmostEqual, math.Exp(-1), 1e-12)
		})

		Convey("Zero progress should leave the value untouched", func() {
			remaining := types.NewInput[float64]()
			So(shape.Write(types.NewValue(0.0)), ShouldBeNil)
			So(shape.Read(remaining), ShouldBeNil)
			So(remaining.Value(), ShouldEqual, 1)
		})

		Convey("Read before Write should fail", func() {
			So(NewExponential().Read(types.NewInput[float64]()), ShouldNotBeNil)
		})

		Convey("Reset should require a new write", func() {
			So(shape.Write(types.NewValue(1.0)), ShouldBeNil)
			So(shape.Reset(), ShouldBeNil)
			So(shape.Read(types.NewInput[float64]()), ShouldNotBeNil)
		})

		Convey("Close should succeed", func() {
			So(shape.Close(), ShouldBeNil)
		})
	})
}
