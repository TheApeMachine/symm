package logic

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/types"
)

func TestGateAndObservationPrimitives(t *testing.T) {
	Convey("Gate passes the value only under a non-zero finite condition", t, func() {
		for condition, expected := range map[float64]float64{0: 0, 1: 7, -1: 7, 0.25: 7} {
			input := types.Frame{}.Set(SymbolCondition, condition).Set(SymbolValue, 7)
			output := Gate(input)
			So(output.Err, ShouldBeNil)
			So(output.MustGet(SymbolResult), ShouldEqual, expected)
		}
		for _, input := range []types.Frame{
			types.Frame{}.Set(SymbolCondition, 1),
			types.Frame{}.Set(SymbolCondition, math.NaN()).Set(SymbolValue, 1),
			types.Frame{}.Set(SymbolCondition, 1).Set(SymbolValue, math.Inf(1)),
		} {
			output := Gate(input)
			So(output.Err, ShouldNotBeNil)
		}
	})

	Convey("Mux selects exactly A or B and rejects poisoned choices", t, func() {
		input := types.Frame{}.
			Set(SymbolCondition, -1).
			Set(calculus.PortA, 3).
			Set(calculus.PortB, 9)
		output := Mux(input)
		So(output.Err, ShouldBeNil)
		So(output.MustGet(SymbolResult), ShouldEqual, 3.0)
		input.Put(SymbolCondition, 0)
		output = Mux(input)
		So(output.Err, ShouldBeNil)
		So(output.MustGet(SymbolResult), ShouldEqual, 9.0)
		input.Put(calculus.PortA, math.NaN())
		output = Mux(input)
		So(output.Err, ShouldNotBeNil)
	})

	Convey("Observe checks presence while EnsureFinite additionally rejects poison", t, func() {
		first := types.MustIntern("test/logic/observe/first")
		second := types.MustIntern("test/logic/observe/second")
		input := types.Frame{}.Set(first, math.NaN()).Set(second, 2)
		output := Observe(first, second)(input)
		So(output.Err, ShouldBeNil)
		So(output.Equal(input), ShouldBeTrue)
		output = Observe(first, second)(types.Frame{}.Set(first, 1))
		So(output.Err, ShouldNotBeNil)
		output = EnsureFinite(first, second)(input)
		So(output.Err, ShouldNotBeNil)
		output = EnsureFinite(first, second)(types.Frame{}.Set(first, 1).Set(second, 2))
		So(output.Err, ShouldBeNil)
		So(output.Count(), ShouldEqual, 2)
	})
}
