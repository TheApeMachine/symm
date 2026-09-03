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
			output := types.Frame{}.Set(SymbolCondition, condition).Set(SymbolValue, 7)
			Gate(&output)
			So(output.Err, ShouldBeNil)
			So(output.MustGet(SymbolResult), ShouldEqual, expected)
		}
		for _, input := range []types.Frame{
			types.Frame{}.Set(SymbolCondition, 1),
			types.Frame{}.Set(SymbolCondition, math.NaN()).Set(SymbolValue, 1),
			types.Frame{}.Set(SymbolCondition, 1).Set(SymbolValue, math.Inf(1)),
		} {
			output := input
			Gate(&output)
			So(output.Err, ShouldNotBeNil)
		}
	})

	Convey("Mux selects exactly A or B and rejects poisoned choices", t, func() {
		input := types.Frame{}.
			Set(SymbolCondition, -1).
			Set(calculus.PortA, 3).
			Set(calculus.PortB, 9)
		output := input
		Mux(&output)
		So(output.Err, ShouldBeNil)
		So(output.MustGet(SymbolResult), ShouldEqual, 3.0)
		input.Put(SymbolCondition, 0)
		output = input
		Mux(&output)
		So(output.Err, ShouldBeNil)
		So(output.MustGet(SymbolResult), ShouldEqual, 9.0)
		input.Put(calculus.PortA, math.NaN())
		output = input
		Mux(&output)
		So(output.Err, ShouldNotBeNil)
	})

	Convey("Observe checks presence while EnsureFinite additionally rejects poison", t, func() {
		first := types.MustIntern("test/logic/observe/first")
		second := types.MustIntern("test/logic/observe/second")
		input := types.Frame{}.Set(first, math.NaN()).Set(second, 2)

		output := input
		Observe(first, second)(&output)
		So(output.Err, ShouldBeNil)
		So(output.Equal(&input), ShouldBeTrue)

		output = types.Frame{}.Set(first, 1)
		Observe(first, second)(&output)
		So(output.Err, ShouldNotBeNil)

		output = input
		EnsureFinite(first, second)(&output)
		So(output.Err, ShouldNotBeNil)

		output = types.Frame{}.Set(first, 1).Set(second, 2)
		EnsureFinite(first, second)(&output)
		So(output.Err, ShouldBeNil)
		So(output.Count(), ShouldEqual, 2)
	})
}
