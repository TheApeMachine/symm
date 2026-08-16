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
		params := types.NewMap[string, types.Value[float64]]()
		params.Put("level", types.NewValue(10.0))
		params.Put("clock", types.NewValue(0.5))

		decay := calculus.NewDecay(types.NewInput(types.NewValue(params)))

		Convey("It should execute write then read through Number", func() {
			out := Number(types.NewInput(types.NewValue(params)), decay)
			So(out.Error(), ShouldBeBlank)

			res, ok := out.Project().Read().Get("result")
			So(ok, ShouldBeTrue)
			So(res.Read(), ShouldEqual, 5.0)
		})
	})

	Convey("Given a level, a clock, and an exponential shape", t, func() {
		clockMap := types.NewMap[string, types.Value[float64]]()
		clockMap.Put("age", types.NewValue(1.0))
		clockMap.Put("span", types.NewValue(2.0))

		clock := temporal.NewClock(types.NewInput(types.NewValue(clockMap)))
		clockOut := Number(types.NewInput(types.NewValue(clockMap)), clock).Project().Read()
		progVal, _ := clockOut.Get("progress")

		exp := calculus.NewExponential(types.NewInput(types.NewValue(progVal.Read())))
		expOut := Number(types.NewInput(types.NewValue(progVal.Read())), exp).Project().Read()

		decayMap := types.NewMap[string, types.Value[float64]]()
		decayMap.Put("level", types.NewValue(10.0))
		decayMap.Put("clock", progVal)
		decayMap.Put("shape", types.NewValue(expOut))

		decay := calculus.NewDecay(types.NewInput(types.NewValue(decayMap)))

		Convey("It should compose decay, shape, and timing", func() {
			out := Number(types.NewInput(types.NewValue(decayMap)), decay)
			So(out.Error(), ShouldBeBlank)

			res, ok := out.Project().Read().Get("result")
			So(ok, ShouldBeTrue)
			So(res.Read(), ShouldAlmostEqual, 10.0*math.Exp(-0.5), 1e-9)
		})
	})
}

func BenchmarkNumber(b *testing.B) {
	decayMap := types.NewMap[string, types.Value[float64]]()
	decayMap.Put("level", types.NewValue(10.0))
	decayMap.Put("clock", types.NewValue(0.5))
	decayMap.Put("shape", types.NewValue(math.Exp(-0.5)))

	decay := calculus.NewDecay(types.NewInput(types.NewValue(decayMap)))
	in := types.NewInput(types.NewValue(decayMap))

	b.ResetTimer()

	for b.Loop() {
		_ = Number(in, decay)
	}
}
