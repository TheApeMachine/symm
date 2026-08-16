package calculus

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/types"
)

func TestDecay(t *testing.T) {
	Convey("Given a decay without clock", t, func() {
		params := types.NewMap[string, types.Value[float64]]()
		params.Put("level", types.NewValue(10.0))

		decay := NewDecay(types.NewInput(types.NewValue(params)))

		Convey("Read should zero out the value in one iteration", func() {
			decay.Write(types.NewInput(types.NewValue(params)))
			out := decay.Read()
			So(out.Error(), ShouldBeBlank)

			res, ok := out.Project().Read().Get("result")
			So(ok, ShouldBeTrue)
			So(res.Read(), ShouldEqual, 0)
		})
	})

	Convey("Given a clock progress of 0.5", t, func() {
		params := types.NewMap[string, types.Value[float64]]()
		params.Put("level", types.NewValue(10.0))
		params.Put("clock", types.NewValue(0.5))

		decay := NewDecay(types.NewInput(types.NewValue(params)))

		Convey("Write then Read should walk linearly toward zero", func() {
			decay.Write(types.NewInput(types.NewValue(params)))
			out := decay.Read()
			So(out.Error(), ShouldBeBlank)

			res, ok := out.Project().Read().Get("result")
			So(ok, ShouldBeTrue)
			So(res.Read(), ShouldEqual, 5.0)
		})

		Convey("An exponential shape should apply e-folding", func() {
			params.Put("shape", types.NewValue(math.Exp(-0.5)))
			decay.Write(types.NewInput(types.NewValue(params)))
			out := decay.Read()
			So(out.Error(), ShouldBeBlank)

			res, ok := out.Project().Read().Get("result")
			So(ok, ShouldBeTrue)
			So(res.Read(), ShouldAlmostEqual, 10.0*math.Exp(-0.5), 1e-9)
		})

		Convey("Close should succeed", func() {
			So(decay.Close(), ShouldBeNil)
		})
	})
}
