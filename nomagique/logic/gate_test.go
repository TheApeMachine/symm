package logic

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/types"
)

func TestGateAndObservationPrimitives(t *testing.T) {
	Convey("Gate passes the value only under a non-zero finite condition", t, func() {
		for condition, expected := range map[float64]float64{0: 0, 1: 7, -1: 7, 0.25: 7} {
			input := types.Frame{}.Set(SymbolCondition, condition).Set(SymbolValue, 7)
			_, output, err := Gate(types.Frame{}, input)
			So(err, ShouldBeNil)
			So(output.MustGet(SymbolResult), ShouldEqual, expected)
		}
		for _, input := range []types.Frame{
			types.Frame{}.Set(SymbolCondition, 1),
			types.Frame{}.Set(SymbolCondition, math.NaN()).Set(SymbolValue, 1),
			types.Frame{}.Set(SymbolCondition, 1).Set(SymbolValue, math.Inf(1)),
		} {
			_, _, err := Gate(types.Frame{}, input)
			So(err, ShouldNotBeNil)
		}
	})

	Convey("Mux selects exactly A or B and rejects poisoned choices", t, func() {
		input := types.Frame{}.
			Set(SymbolCondition, -1).
			Set(calculus.PortA, 3).
			Set(calculus.PortB, 9)
		_, output, err := Mux(types.Frame{}, input)
		So(err, ShouldBeNil)
		So(output.MustGet(SymbolResult), ShouldEqual, 3.0)
		input.Put(SymbolCondition, 0)
		_, output, err = Mux(types.Frame{}, input)
		So(err, ShouldBeNil)
		So(output.MustGet(SymbolResult), ShouldEqual, 9.0)
		input.Put(calculus.PortA, math.NaN())
		_, _, err = Mux(types.Frame{}, input)
		So(err, ShouldNotBeNil)
	})

	Convey("Observe checks presence while EnsureFinite additionally rejects poison", t, func() {
		first := nomagique.MustIntern("test/logic/observe/first")
		second := nomagique.MustIntern("test/logic/observe/second")
		input := types.Frame{}.Set(first, math.NaN()).Set(second, 2)
		_, output, err := Observe(first, second)(types.Frame{}, input)
		So(err, ShouldBeNil)
		So(output.Equal(input), ShouldBeTrue)
		_, _, err = Observe(first, second)(types.Frame{}, types.Frame{}.Set(first, 1))
		So(err, ShouldNotBeNil)
		_, _, err = EnsureFinite(first, second)(types.Frame{}, input)
		So(err, ShouldNotBeNil)
		_, output, err = EnsureFinite(first, second)(types.Frame{}, types.Frame{}.Set(first, 1).Set(second, 2))
		So(err, ShouldBeNil)
		So(output.Count(), ShouldEqual, 2)
	})
}
