package logic

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/types"
)

func TestComparisonPrimitives(t *testing.T) {
	input := func(a, b float64) types.Frame {
		return types.Frame{}.Set(calculus.PortA, a).Set(calculus.PortB, b)
	}

	Convey("Comparison atoms emit exact binary conditions", t, func() {
		cases := []struct {
			primitive types.Primitive
			a, b      float64
			expected  float64
		}{
			{GreaterThan, 2, 1, 1}, {GreaterThan, 1, 1, 0}, {GreaterThan, -2, -1, 0},
			{GreaterOrEqual, 2, 1, 1}, {GreaterOrEqual, 1, 1, 1}, {GreaterOrEqual, -2, -1, 0},
			{LessThan, 2, 1, 0}, {LessThan, 1, 1, 0}, {LessThan, -2, -1, 1},
			{Equal, 2, 1, 0}, {Equal, 1, 1, 1}, {Equal, 0, math.Copysign(0, -1), 1},
		}
		for _, test := range cases {
			output := input(test.a, test.b)
			test.primitive(&output)
			So(output.Err, ShouldBeNil)
			So(output.MustGet(SymbolCondition), ShouldEqual, test.expected)
		}
	})

	Convey("Comparisons reject ambiguous non-finite ordering", t, func() {
		for _, primitive := range []types.Primitive{GreaterThan, GreaterOrEqual, LessThan, Equal} {
			for _, bad := range []types.Frame{
				types.Frame{}, input(math.NaN(), 1), input(1, math.Inf(1)),
			} {
				output := bad
				primitive(&output)
				So(output.Err, ShouldNotBeNil)
			}
		}
	})

	Convey("PositiveOrder enforces the full strict invariant", t, func() {
		lower := types.MustIntern("test/logic/lower")
		upper := types.MustIntern("test/logic/upper")
		validate := PositiveOrder(lower, upper)
		output := types.Frame{}.Set(lower, 1).Set(upper, 2)
		validate(&output)
		So(output.Err, ShouldBeNil)
		So(output.MustGet(lower), ShouldEqual, 1.0)
		for _, pair := range [][2]float64{{0, 1}, {-1, 1}, {2, 2}, {3, 2}, {math.NaN(), 2}, {1, math.Inf(1)}} {
			output = types.Frame{}.Set(lower, pair[0]).Set(upper, pair[1])
			validate(&output)
			So(output.Err, ShouldNotBeNil)
		}
	})
}
