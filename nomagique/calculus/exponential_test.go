package calculus

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/types"
)

func TestExponential(t *testing.T) {
	Convey("Given an Exponential primitive", t, func() {
		exponential := NewExponential(types.NewInput(types.NewValue(0.0)))

		Convey("Progress 0 should yield 1.0", func() {
			exponential.Write(types.NewInput(types.NewValue(0.0)))
			out := exponential.Read()
			So(out.Error(), ShouldBeBlank)
			So(out.Project().Read(), ShouldEqual, 1.0)
		})

		Convey("Progress 1.0 should yield e^(-1)", func() {
			exponential.Write(types.NewInput(types.NewValue(1.0)))
			out := exponential.Read()
			So(out.Error(), ShouldBeBlank)
			So(out.Project().Read(), ShouldAlmostEqual, math.Exp(-1.0), 1e-9)
		})

		Convey("Close should succeed", func() {
			So(exponential.Close(), ShouldBeNil)
		})
	})
}
