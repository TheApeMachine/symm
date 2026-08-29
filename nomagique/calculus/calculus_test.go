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
			output := test.input
			types.Step(test.primitive, &output)
			So(output.Err, ShouldBeNil)
			So(output.MustGet(PortResult), ShouldEqual, test.expected)
			So(output.Count(), ShouldEqual, test.input.Count()+1)
		}
	})

	Convey("Average resists addition overflow at equal extremes", t, func() {
		output := binary(math.MaxFloat64, math.MaxFloat64)
		Average(&output)
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
			types.Step(test.primitive, &merged)
			So(merged.Err, ShouldNotBeNil)
			So(merged.MustGet(SymbolTotal), ShouldEqual, 4)
			So(merged.Has(PortResult), ShouldBeFalse)
		}
	})

	Convey("Rate accepts signed counts but requires positive elapsed duration", t, func() {
		output := types.Frame{}.Set(SymbolCount, -6).Set(SymbolDuration, 3)
		Rate(&output)
		So(output.Err, ShouldBeNil)
		So(output.MustGet(SymbolRate), ShouldEqual, -2.0)
		for _, duration := range []float64{0, -1, math.NaN(), math.Inf(1)} {
			output = types.Frame{}.
				Set(SymbolCount, 1).
				Set(SymbolDuration, duration)
			Rate(&output)
			So(output.Err, ShouldNotBeNil)
		}
	})

	Convey("Attack publishes the same finite result through base and result", t, func() {
		output := types.Frame{}.Set(SymbolBase, 4).Set(SymbolJump, -1.5)
		Attack(&output)
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
		output := frame(4, 2, 10)
		Extent(&output)
		So(output.Err, ShouldBeNil)
		So(output.MustGet(PortResult), ShouldEqual, 8.0)

		output = frame(4, 2, 10)
		Center(&output)
		So(output.Err, ShouldBeNil)
		So(output.MustGet(PortResult), ShouldEqual, 6.0)

		output = frame(4, 2, 10)
		Project(&output)
		So(output.Err, ShouldBeNil)
		So(output.MustGet(PortResult), ShouldEqual, 0.25)

		output = frame(4, 2, 10)
		Reflect(&output)
		So(output.Err, ShouldBeNil)
		So(output.MustGet(PortResult), ShouldEqual, 8.0)
	})

	Convey("Project rejects a collapsed coordinate system", t, func() {
		output := frame(4, 2, 2)
		Project(&output)
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
			output := frame(x, 2, 10)
			Coherence(&output)
			So(output.Err, ShouldBeNil)
			So(output.MustGet(PortResult), ShouldEqual, expected)
		}
		output := frame(999, 3, 3)
		Coherence(&output)
		So(output.Err, ShouldBeNil)
		So(output.MustGet(PortResult), ShouldEqual, 1.0)
	})
}

func TestNonlinearPrimitives(t *testing.T) {
	Convey("LogRatio is defined only for positive finite observations", t, func() {
		output := types.Frame{}.Set(SymbolCurrent, 4).Set(SymbolPrevious, 2)
		LogRatio(&output)
		So(output.Err, ShouldBeNil)
		So(output.MustGet(PortResult), ShouldAlmostEqual, math.Log(2.0), 1e-12)
		for _, pair := range [][2]float64{{0, 1}, {-1, 1}, {1, 0}, {math.NaN(), 1}, {1, math.Inf(1)}} {
			output = types.Frame{}.
				Set(SymbolCurrent, pair[0]).Set(SymbolPrevious, pair[1])
			LogRatio(&output)
			So(output.Err, ShouldNotBeNil)
		}
	})

	Convey("Squash is bounded, sign symmetric, and explicit at zero scale", t, func() {
		for _, value := range []float64{-100, -1, 0, 1, 100} {
			output := types.Frame{}.Set(PortX, value).Set(SymbolScale, 2)
			Squash(&output)
			So(output.Err, ShouldBeNil)
			result := output.MustGet(PortResult)
			So(result, ShouldBeBetweenOrEqual, -1.0, 1.0)
			mirror := types.Frame{}.Set(PortX, -value).Set(SymbolScale, 2)
			Squash(&mirror)
			So(mirror.Err, ShouldBeNil)
			So(mirror.MustGet(PortResult), ShouldAlmostEqual, -result, 1e-12)
		}
		zero := types.Frame{}.
			Set(PortX, 0).Set(SymbolScale, 0)
		Squash(&zero)
		So(zero.Err, ShouldBeNil)
		So(zero.MustGet(PortResult), ShouldEqual, 0.0)
		negScale := types.Frame{}.
			Set(PortX, 1).Set(SymbolScale, -1)
		Squash(&negScale)
		So(negScale.Err, ShouldBeNil)
		So(negScale.MustGet(PortResult), ShouldAlmostEqual, 0.5, 1e-12)
	})
}

func TestStatefulCalculusPrimitives(t *testing.T) {
	Convey("Accumulate consumes only the explicit delta port", t, func() {
		state := types.Frame{}.Set(SymbolTotal, 5)
		output := state.Merged(types.Frame{}.Set(SymbolDelta, 2))
		Accumulate(&output)
		So(output.Err, ShouldBeNil)
		So(output.MustGet(SymbolTotal), ShouldEqual, 7.0)
		So(output.MustGet(PortResult), ShouldEqual, 7.0)

		output = state.Merged(types.Frame{}.Set(PortX, 2))
		Accumulate(&output)
		So(output.Err, ShouldNotBeNil)
	})

	Convey("Accumulate rejects poisoned state and overflow without committing", t, func() {
		for _, stateValue := range []float64{math.NaN(), math.Inf(1)} {
			initial := types.Frame{}.Set(SymbolTotal, stateValue)
			merged := initial.Merged(types.Frame{}.Set(SymbolDelta, 1))
			types.Step(Accumulate, &merged)
			So(merged.Err, ShouldNotBeNil)
			So(math.Float64bits(merged.MustGet(SymbolTotal)), ShouldEqual, math.Float64bits(stateValue))
			So(merged.Has(PortResult), ShouldBeFalse)
		}
		initial := types.Frame{}.Set(SymbolTotal, math.MaxFloat64)
		merged := initial.Merged(types.Frame{}.Set(SymbolDelta, math.MaxFloat64))
		types.Step(Accumulate, &merged)
		So(merged.Err, ShouldNotBeNil)
		So(math.Float64bits(merged.MustGet(SymbolTotal)), ShouldEqual, math.Float64bits(math.MaxFloat64))
		So(merged.Has(PortResult), ShouldBeFalse)
	})

	Convey("Clear snapshots its configuration and changes only state", t, func() {
		first := types.MustIntern("test/calculus/clear/first")
		second := types.MustIntern("test/calculus/clear/second")
		symbols := []types.Symbol{first}
		clear := Clear(symbols...)
		symbols[0] = second
		state := types.Frame{}.Set(first, 1).Set(second, 2)
		merged := state.Merged(types.Frame{}.Set(PortX, 3))
		clear(&merged)
		So(merged.Err, ShouldBeNil)
		So(merged.Has(first), ShouldBeFalse)
		So(merged.MustGet(second), ShouldEqual, 2.0)
		So(merged.MustGet(PortX), ShouldEqual, 3.0)
	})

	Convey("Decay applies an explicit clock or shape and rejects poison", t, func() {
		state := types.Frame{}.Set(SymbolLevel, 10)
		output := state.Merged(types.Frame{}.Set(SymbolClock, 0.25))
		Decay(&output)
		So(output.Err, ShouldBeNil)
		So(output.MustGet(SymbolLevel), ShouldEqual, 7.5)
		So(output.MustGet(PortResult), ShouldEqual, 7.5)
		output = state.Merged(types.Frame{}.Set(SymbolShape, 0.2))
		Decay(&output)
		So(output.Err, ShouldBeNil)
		So(output.MustGet(PortResult), ShouldEqual, 2.0)
		output = state.Merged(types.Frame{}.Set(SymbolShape, math.NaN()))
		Decay(&output)
		So(output.Err, ShouldNotBeNil)
	})
}
