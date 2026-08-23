package logic

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/types"
)

func TestBooleanPrimitives(t *testing.T) {
	input := func(a, b float64) types.Frame {
		return types.Frame{}.Set(calculus.PortA, a).Set(calculus.PortB, b)
	}

	Convey("And, Or, and Xor obey their complete numeric truth tables", t, func() {
		cases := []struct {
			primitive types.Primitive
			a, b      float64
			expected  float64
		}{
			{And, 0, 0, 0}, {And, 0, 2, 0}, {And, -1, 0, 0}, {And, -1, 2, 1},
			{Or, 0, 0, 0}, {Or, 0, 2, 1}, {Or, -1, 0, 1}, {Or, -1, 2, 1},
			{Xor, 0, 0, 0}, {Xor, 0, 2, 1}, {Xor, -1, 0, 1}, {Xor, -1, 2, 0},
		}
		for _, test := range cases {
			_, output, err := test.primitive(types.Frame{}, input(test.a, test.b))
			So(err, ShouldBeNil)
			So(output.MustGet(SymbolCondition), ShouldEqual, test.expected)
			So(output.MustGet(SymbolResult), ShouldEqual, test.expected)
		}
	})

	Convey("Not treats exactly zero as false", t, func() {
		for condition, expected := range map[float64]float64{0: 1, 1: 0, -1: 0, 0.5: 0} {
			_, output, err := Not(types.Frame{}, types.Frame{}.Set(SymbolCondition, condition))
			So(err, ShouldBeNil)
			So(output.MustGet(SymbolResult), ShouldEqual, expected)
		}
	})

	Convey("Boolean atoms reject absence, NaN, and infinities", t, func() {
		for _, primitive := range []types.Primitive{And, Or, Xor} {
			for _, bad := range []types.Frame{
				types.Frame{}.Set(calculus.PortA, 1),
				input(math.NaN(), 1),
				input(1, math.Inf(1)),
			} {
				_, output, err := primitive(types.Frame{}, bad)
				So(err, ShouldNotBeNil)
				So(output.Count(), ShouldEqual, 0)
			}
		}
		for _, condition := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
			_, _, err := Not(types.Frame{}, types.Frame{}.Set(SymbolCondition, condition))
			So(err, ShouldNotBeNil)
		}
	})
}
