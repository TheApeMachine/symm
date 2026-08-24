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
			output := types.Step(test.primitive, test.input)
			So(output.Err, ShouldBeNil)
			So(output.MustGet(PortResult), ShouldEqual, test.expected)
			So(output.Count(), ShouldEqual, test.input.Count()+1)
		}
	})

	Convey("Average resists addition overflow at equal extremes", t, func() {
		output := Average(binary(math.MaxFloat64, math.MaxFloat64))
		So(output.Err, ShouldBeNil)
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
			merged := initial.Merged(test.input)
			output := types.Step(test.primitive, merged)
			So(output.Err, ShouldNotBeNil)
			So(output.MustGet(SymbolTotal), ShouldEqual, 4)
			So(output.Has(PortResult), ShouldBeFalse)
		}
	})

	Convey("Rate accepts signed counts but requires positive elapsed duration", t, func() {
		input := types.Frame{}.Set(SymbolCount, -6).Set(SymbolDuration, 3)
		output := Rate(input)
		So(output.Err, ShouldBeNil)
		So(output.MustGet(SymbolRate), ShouldEqual, -2.0)
		for _, duration := range []float64{0, -1, math.NaN(), math.Inf(1)} {
			output = Rate(types.Frame{}.
				Set(SymbolCount, 1).
				Set(SymbolDuration, duration))
			So(output.Err, ShouldNotBeNil)
		}
	})

	Convey("Attack publishes the same finite result through base and result", t, func() {
		input := types.Frame{}.Set(SymbolBase, 4).Set(SymbolJump, -1.5)
		output := Attack(input)
		So(output.Err, ShouldBeNil)
		So(output.MustGet(SymbolBase), ShouldEqual, 2.5)
		So(output.MustGet(PortResult), ShouldEqual, 2.5)
	})
}

func TestAffinePrimitives(t *testing.T) {
	frame := func(x, a, b float64) types.Frame {
		return types.Frame{}.Set(PortX, x).Set(PortA, a).Set(PortB, b)
	}

	Convey("Extent, Center, Project, and Reflect form explicit affine atoms", t, func() {
		output := Extent(frame(4, 2, 10))
		So(output.Err, ShouldBeNil)
		So(output.MustGet(PortResult), ShouldEqual, 8.0)

		output = Center(frame(4, 2, 10))
		So(output.Err, ShouldBeNil)
		So(output.MustGet(PortResult), ShouldEqual, 6.0)

		output = Project(frame(4, 2, 10))
		So(output.Err, ShouldBeNil)
		So(output.MustGet(PortResult), ShouldEqual, 0.25)

		output = Reflect(frame(4, 2, 10))
		So(output.Err, ShouldBeNil)
		So(output.MustGet(PortResult), ShouldEqual, 8.0)
	})

	Convey("Project rejects a collapsed coordinate system", t, func() {
		output := Project(frame(4, 2, 2))
		So(output.Err, ShouldNotBeNil)
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
			output := Coherence(frame(x, 2, 10))
			So(output.Err, ShouldBeNil)
			So(output.MustGet(PortResult), ShouldEqual, expected)
		}
		output := Coherence(frame(999, 3, 3))
		So(output.Err, ShouldBeNil)
		So(output.MustGet(PortResult), ShouldEqual, 1.0)
	})
}

func TestNonlinearPrimitives(t *testing.T) {
	Convey("LogRatio is defined only for positive finite observations", t, func() {
		input := types.Frame{}.Set(SymbolCurrent, 4).Set(SymbolPrevious, 2)
		output := LogRatio(input)
		So(output.Err, ShouldBeNil)
		So(output.MustGet(PortResult), ShouldAlmostEqual, math.Log(2.0), 1e-12)
		for _, pair := range [][2]float64{{0, 1}, {-1, 1}, {1, 0}, {math.NaN(), 1}, {1, math.Inf(1)}} {
			output = LogRatio(types.Frame{}.
				Set(SymbolCurrent, pair[0]).Set(SymbolPrevious, pair[1]))
			So(output.Err, ShouldNotBeNil)
		}
	})

	Convey("Squash is bounded, sign symmetric, and explicit at zero scale", t, func() {
		for _, value := range []float64{-100, -1, 0, 1, 100} {
			input := types.Frame{}.Set(PortX, value).Set(SymbolScale, 2)
			output := Squash(input)
			So(output.Err, ShouldBeNil)
			result := output.MustGet(PortResult)
			So(result, ShouldBeBetweenOrEqual, -1.0, 1.0)
			mirrorInput := types.Frame{}.Set(PortX, -value).Set(SymbolScale, 2)
			mirror := Squash(mirrorInput)
			So(mirror.Err, ShouldBeNil)
			So(mirror.MustGet(PortResult), ShouldAlmostEqual, -result, 1e-12)
		}
		zero := Squash(types.Frame{}.
			Set(PortX, 0).Set(SymbolScale, 0))
		So(zero.Err, ShouldBeNil)
		So(zero.MustGet(PortResult), ShouldEqual, 0.0)
		negScale := Squash(types.Frame{}.
			Set(PortX, 1).Set(SymbolScale, -1))
		So(negScale.Err, ShouldBeNil)
		So(negScale.MustGet(PortResult), ShouldAlmostEqual, 0.5, 1e-12)
	})
}

func TestStatefulCalculusPrimitives(t *testing.T) {
	Convey("Accumulate consumes only the explicit delta port", t, func() {
		state := types.Frame{}.Set(SymbolTotal, 5)
		output := Accumulate(state.Merged(types.Frame{}.Set(SymbolDelta, 2)))
		So(output.Err, ShouldBeNil)
		So(output.MustGet(SymbolTotal), ShouldEqual, 7.0)
		So(output.MustGet(PortResult), ShouldEqual, 7.0)

		output = Accumulate(state.Merged(types.Frame{}.Set(PortX, 2)))
		So(output.Err, ShouldNotBeNil)
	})

	Convey("Accumulate rejects poisoned state and overflow without committing", t, func() {
		for _, stateValue := range []float64{math.NaN(), math.Inf(1)} {
			initial := types.Frame{}.Set(SymbolTotal, stateValue)
			merged := initial.Merged(types.Frame{}.Set(SymbolDelta, 1))
			output := types.Step(Accumulate, merged)
			So(output.Err, ShouldNotBeNil)
			So(math.Float64bits(output.MustGet(SymbolTotal)), ShouldEqual, math.Float64bits(stateValue))
			So(output.Has(PortResult), ShouldBeFalse)
		}
		initial := types.Frame{}.Set(SymbolTotal, math.MaxFloat64)
		merged := initial.Merged(types.Frame{}.Set(SymbolDelta, math.MaxFloat64))
		output := types.Step(Accumulate, merged)
		So(output.Err, ShouldNotBeNil)
		So(math.Float64bits(output.MustGet(SymbolTotal)), ShouldEqual, math.Float64bits(math.MaxFloat64))
		So(output.Has(PortResult), ShouldBeFalse)
	})

	Convey("Clear snapshots its configuration and changes only state", t, func() {
		first := types.MustIntern("test/calculus/clear/first")
		second := types.MustIntern("test/calculus/clear/second")
		symbols := []types.Symbol{first}
		clear := Clear(symbols...)
		symbols[0] = second
		state := types.Frame{}.Set(first, 1).Set(second, 2)
		merged := state.Merged(types.Frame{}.Set(PortX, 3))
		output := clear(merged)
		So(output.Err, ShouldBeNil)
		So(output.Has(first), ShouldBeFalse)
		So(output.MustGet(second), ShouldEqual, 2.0)
		So(output.MustGet(PortX), ShouldEqual, 3.0)
	})

	Convey("Decay applies an explicit clock or shape and rejects poison", t, func() {
		state := types.Frame{}.Set(SymbolLevel, 10)
		output := Decay(state.Merged(types.Frame{}.Set(SymbolClock, 0.25)))
		So(output.Err, ShouldBeNil)
		So(output.MustGet(SymbolLevel), ShouldEqual, 7.5)
		So(output.MustGet(PortResult), ShouldEqual, 7.5)
		output = Decay(state.Merged(types.Frame{}.Set(SymbolShape, 0.2)))
		So(output.Err, ShouldBeNil)
		So(output.MustGet(PortResult), ShouldEqual, 2.0)
		output = Decay(state.Merged(types.Frame{}.Set(SymbolShape, math.NaN())))
		So(output.Err, ShouldNotBeNil)
	})
}
