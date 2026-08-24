package calculus

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/types"
)

func TestArithmeticPrimitives(t *testing.T) {
	binary := func(a, b float64) types.Frame {
		return types.Frame{}.Set(PortA, a).Set(PortB, b)
	}

	Convey("Arithmetic atoms implement one explicit finite relation", t, func() {
		cases := []struct {
			name      string
			primitive types.Primitive
			input     types.Frame
			expected  float64
		}{
			{"sum", Sum, binary(7, -2), 5},
			{"difference", Difference, binary(7, -2), 9},
			{"product", Product, binary(7, -2), -14},
			{"quotient", Quotient, binary(7, -2), -3.5},
			{"average", Average, binary(7, -3), 2},
			{"absolute", Absolute, types.Frame{}.Set(PortX, -8), 8},
			{"negative", Negative, types.Frame{}.Set(PortX, 8), -8},
			{"positive positive", Positive, types.Frame{}.Set(PortX, 8), 8},
			{"positive negative", Positive, types.Frame{}.Set(PortX, -8), 0},
		}

		for _, test := range cases {
			_, output, err := types.Step(test.primitive, types.Frame{}, test.input)
			So(err, ShouldBeNil)
			So(output.MustGet(PortResult), ShouldEqual, test.expected)
			So(output.Count(), ShouldEqual, test.input.Count()+1)
		}
	})

	Convey("Average resists addition overflow at equal extremes", t, func() {
		_, output, err := Average(types.Frame{}, binary(math.MaxFloat64, math.MaxFloat64))
		So(err, ShouldBeNil)
		So(output.MustGet(PortResult), ShouldEqual, math.MaxFloat64)
	})

	Convey("Undefined and overflowing arithmetic is rejected, never converted to a sentinel", t, func() {
		adversarial := []struct {
			primitive types.Primitive
			input     types.Frame
		}{
			{Sum, types.Frame{}.Set(PortA, 1)},
			{Sum, binary(math.NaN(), 1)},
			{Difference, binary(math.Inf(1), 1)},
			{Sum, binary(math.MaxFloat64, math.MaxFloat64)},
			{Product, binary(math.MaxFloat64, 2)},
			{Quotient, binary(1, 0)},
			{Quotient, binary(math.SmallestNonzeroFloat64, math.SmallestNonzeroFloat64/2)},
			{Absolute, types.Frame{}.Set(PortX, math.NaN())},
			{Negative, types.Frame{}.Set(PortX, math.Inf(-1))},
		}

		for _, test := range adversarial {
			initial := types.Frame{}.Set(SymbolTotal, 4)
			next, output, err := types.Step(test.primitive, initial, test.input)
			So(err, ShouldNotBeNil)
			So(next.Equal(initial), ShouldBeTrue)
			So(output.Count(), ShouldEqual, 0)
		}
	})

	Convey("Rate accepts signed counts but requires positive elapsed duration", t, func() {
		input := types.Frame{}.Set(SymbolCount, -6).Set(SymbolDuration, 3)
		_, output, err := Rate(types.Frame{}, input)
		So(err, ShouldBeNil)
		So(output.MustGet(SymbolRate), ShouldEqual, -2.0)
		for _, duration := range []float64{0, -1, math.NaN(), math.Inf(1)} {
			_, _, err = Rate(types.Frame{}, types.Frame{}.
				Set(SymbolCount, 1).
				Set(SymbolDuration, duration))
			So(err, ShouldNotBeNil)
		}
	})

	Convey("Attack publishes the same finite result through base and result", t, func() {
		input := types.Frame{}.Set(SymbolBase, 4).Set(SymbolJump, -1.5)
		_, output, err := Attack(types.Frame{}, input)
		So(err, ShouldBeNil)
		So(output.MustGet(SymbolBase), ShouldEqual, 2.5)
		So(output.MustGet(PortResult), ShouldEqual, 2.5)
	})
}

func TestAffinePrimitives(t *testing.T) {
	frame := func(x, a, b float64) types.Frame {
		return types.Frame{}.Set(PortX, x).Set(PortA, a).Set(PortB, b)
	}

	Convey("Extent, Center, Project, and Reflect form explicit affine atoms", t, func() {
		_, output, err := Extent(types.Frame{}, frame(4, 2, 10))
		So(err, ShouldBeNil)
		So(output.MustGet(PortResult), ShouldEqual, 8.0)

		_, output, err = Center(types.Frame{}, frame(4, 2, 10))
		So(err, ShouldBeNil)
		So(output.MustGet(PortResult), ShouldEqual, 6.0)

		_, output, err = Project(types.Frame{}, frame(4, 2, 10))
		So(err, ShouldBeNil)
		So(output.MustGet(PortResult), ShouldEqual, 0.25)

		_, output, err = Reflect(types.Frame{}, frame(4, 2, 10))
		So(err, ShouldBeNil)
		So(output.MustGet(PortResult), ShouldEqual, 8.0)
	})

	Convey("Project rejects a collapsed coordinate system", t, func() {
		_, _, err := Project(types.Frame{}, frame(4, 2, 2))
		So(err, ShouldNotBeNil)
	})

	Convey("Coherence is central, bounded, symmetric, and defined for a point interval", t, func() {
		for x, expected := range map[float64]float64{
			0:  0,
			2:  0,
			4:  0.5,
			6:  1,
			8:  0.5,
			10: 0,
			12: 0,
		} {
			_, output, err := Coherence(types.Frame{}, frame(x, 2, 10))
			So(err, ShouldBeNil)
			So(output.MustGet(PortResult), ShouldEqual, expected)
		}
		_, output, err := Coherence(types.Frame{}, frame(999, 3, 3))
		So(err, ShouldBeNil)
		So(output.MustGet(PortResult), ShouldEqual, 1.0)
	})
}

func TestNonlinearPrimitives(t *testing.T) {
	Convey("LogRatio is defined only for positive finite observations", t, func() {
		input := types.Frame{}.Set(SymbolCurrent, 4).Set(SymbolPrevious, 2)
		_, output, err := LogRatio(types.Frame{}, input)
		So(err, ShouldBeNil)
		So(output.MustGet(PortResult), ShouldAlmostEqual, math.Log(2.0), 1e-12)
		for _, pair := range [][2]float64{{0, 1}, {-1, 1}, {1, 0}, {math.NaN(), 1}, {1, math.Inf(1)}} {
			_, _, err = LogRatio(types.Frame{}, types.Frame{}.
				Set(SymbolCurrent, pair[0]).Set(SymbolPrevious, pair[1]))
			So(err, ShouldNotBeNil)
		}
	})

	Convey("Squash is bounded, sign symmetric, and explicit at zero scale", t, func() {
		for _, value := range []float64{-100, -1, 0, 1, 100} {
			input := types.Frame{}.Set(PortX, value).Set(SymbolScale, 2)
			_, output, err := Squash(types.Frame{}, input)
			So(err, ShouldBeNil)
			result := output.MustGet(PortResult)
			So(result, ShouldBeBetweenOrEqual, -1.0, 1.0)
			mirrorInput := types.Frame{}.Set(PortX, -value).Set(SymbolScale, 2)
			_, mirror, mirrorErr := Squash(types.Frame{}, mirrorInput)
			So(mirrorErr, ShouldBeNil)
			So(mirror.MustGet(PortResult), ShouldAlmostEqual, -result, 1e-12)
		}
		_, zero, err := Squash(types.Frame{}, types.Frame{}.
			Set(PortX, 0).Set(SymbolScale, 0))
		So(err, ShouldBeNil)
		So(zero.MustGet(PortResult), ShouldEqual, 0.0)
		_, negScale, err := Squash(types.Frame{}, types.Frame{}.
			Set(PortX, 1).Set(SymbolScale, -1))
		So(err, ShouldBeNil)
		So(negScale.MustGet(PortResult), ShouldAlmostEqual, 0.5, 1e-12)
	})
}

func TestStatefulCalculusPrimitives(t *testing.T) {
	Convey("Accumulate consumes only the explicit delta port", t, func() {
		state := types.Frame{}.Set(SymbolTotal, 5)
		next, output, err := Accumulate(state, types.Frame{}.Set(SymbolDelta, 2))
		So(err, ShouldBeNil)
		So(next.MustGet(SymbolTotal), ShouldEqual, 7.0)
		So(output.MustGet(PortResult), ShouldEqual, 7.0)

		_, _, err = Accumulate(state, types.Frame{}.Set(PortX, 2))
		So(err, ShouldNotBeNil)
	})

	Convey("Accumulate rejects poisoned state and overflow without committing", t, func() {
		for _, stateValue := range []float64{math.NaN(), math.Inf(1)} {
			initial := types.Frame{}.Set(SymbolTotal, stateValue)
			next, output, err := types.Step(Accumulate, initial, types.Frame{}.Set(SymbolDelta, 1))
			So(err, ShouldNotBeNil)
			So(next.Equal(initial), ShouldBeTrue)
			So(output.Count(), ShouldEqual, 0)
		}
		initial := types.Frame{}.Set(SymbolTotal, math.MaxFloat64)
		next, _, err := types.Step(Accumulate, initial, types.Frame{}.Set(SymbolDelta, math.MaxFloat64))
		So(err, ShouldNotBeNil)
		So(next.Equal(initial), ShouldBeTrue)
	})

	Convey("Clear snapshots its configuration and changes only state", t, func() {
		first := types.MustIntern("test/calculus/clear/first")
		second := types.MustIntern("test/calculus/clear/second")
		symbols := []types.Symbol{first}
		clear := Clear(symbols...)
		symbols[0] = second
		state := types.Frame{}.Set(first, 1).Set(second, 2)
		next, output, err := clear(state, types.Frame{}.Set(PortX, 3))
		So(err, ShouldBeNil)
		So(next.Has(first), ShouldBeFalse)
		So(next.MustGet(second), ShouldEqual, 2.0)
		So(output.MustGet(PortX), ShouldEqual, 3.0)
	})

	Convey("Decay applies an explicit clock or shape and rejects poison", t, func() {
		state := types.Frame{}.Set(SymbolLevel, 10)
		next, output, err := Decay(state, types.Frame{}.Set(SymbolClock, 0.25))
		So(err, ShouldBeNil)
		So(next.MustGet(SymbolLevel), ShouldEqual, 7.5)
		So(output.MustGet(PortResult), ShouldEqual, 7.5)
		_, output, err = Decay(state, types.Frame{}.Set(SymbolShape, 0.2))
		So(err, ShouldBeNil)
		So(output.MustGet(PortResult), ShouldEqual, 2.0)
		_, _, err = Decay(state, types.Frame{}.Set(SymbolShape, math.NaN()))
		So(err, ShouldNotBeNil)
	})
}
