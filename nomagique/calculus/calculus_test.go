package calculus

import (
	"math"
	"testing"

	"github.com/theapemachine/symm/nomagique"
)

func TestScalarPrimitives(t *testing.T) {
	testCases := []struct {
		name      string
		primitive nomagique.Primitive
		input     nomagique.Frame
		result    nomagique.Symbol
		want      float64
	}{
		{name: "sum", primitive: Sum, input: binaryInput(3, 4), result: SymbolResult, want: 7},
		{name: "difference", primitive: Difference, input: binaryInput(3, 4), result: SymbolResult, want: -1},
		{name: "product", primitive: Product, input: binaryInput(3, 4), result: SymbolResult, want: 12},
		{name: "positive", primitive: Positive, input: unaryInput(-3), result: SymbolResult, want: 0},
		{name: "log ratio", primitive: LogRatio, input: ratioInput(110, 100), result: SymbolResult, want: math.Log(1.1)},
		{name: "squash", primitive: Squash, input: scaleInput(2, 2), result: SymbolResult, want: 0.5},
		{name: "inverse", primitive: Inverse, input: scaleInput(2, 2), result: SymbolResult, want: 0.5},
		{name: "ratio", primitive: Ratio, input: normalizedInput(2, 4, 1), result: SymbolResult, want: 0.5},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			_, output, err := testCase.primitive(nomagique.Frame{}, testCase.input)

			if err != nil {
				t.Fatal(err)
			}

			got := output.MustGet(testCase.result)

			if math.Abs(got-testCase.want) > 1e-12 {
				t.Fatalf("result=%v; want %v", got, testCase.want)
			}
		})
	}
}

func TestAccumulateAndDecayRetainState(t *testing.T) {
	accumulator := nomagique.NewStream(Accumulate, nomagique.Frame{})
	input := nomagique.Frame{}
	input.Put(SymbolDelta, 3)

	if _, err := accumulator.Step(input); err != nil {
		t.Fatal(err)
	}

	input.Put(SymbolDelta, 4)
	output, err := accumulator.Step(input)

	if err != nil {
		t.Fatal(err)
	}

	if got := output.MustGet(SymbolResult); got != 7 {
		t.Fatalf("accumulated=%v; want 7", got)
	}

	decayState := nomagique.Frame{}
	decayState.Put(SymbolLevel, 10)
	decayInput := nomagique.Frame{}
	decayInput.Put(SymbolClock, 0.5)
	nextState, decayOutput, err := Decay(decayState, decayInput)

	if err != nil {
		t.Fatal(err)
	}

	if got := decayOutput.MustGet(SymbolResult); got != 5 {
		t.Fatalf("decayed=%v; want 5", got)
	}

	if got := nextState.MustGet(SymbolLevel); got != 5 {
		t.Fatalf("state level=%v; want 5", got)
	}
}

func TestBoundarySemantics(t *testing.T) {
	_, inverse, err := Inverse(nomagique.Frame{}, scaleInput(0, 0))

	if err != nil || inverse.MustGet(SymbolResult) != 1 {
		t.Fatalf("inverse absence=%v err=%v; want 1", inverse.MustGet(SymbolResult), err)
	}

	_, ratio, err := Ratio(nomagique.Frame{}, normalizedInput(2, 4, 0))

	if err != nil || ratio.MustGet(SymbolResult) != 0 {
		t.Fatalf("unready ratio=%v err=%v; want 0", ratio.MustGet(SymbolResult), err)
	}

	_, squash, err := Squash(nomagique.Frame{}, scaleInput(2, 0))

	if err != nil || squash.MustGet(SymbolResult) != 0 {
		t.Fatalf("unscaled squash=%v err=%v; want 0", squash.MustGet(SymbolResult), err)
	}
}

func BenchmarkSum(b *testing.B) {
	input := binaryInput(3, 4)
	state := nomagique.Frame{}

	b.ReportAllocs()
	b.ResetTimer()

	for iteration := 0; iteration < b.N; iteration++ {
		_, _, _ = Sum(state, input)
	}
}

func binaryInput(left float64, right float64) nomagique.Frame {
	input := nomagique.Frame{}
	input.Put(SymbolLeft, left)
	input.Put(SymbolRight, right)

	return input
}

func unaryInput(value float64) nomagique.Frame {
	input := nomagique.Frame{}
	input.Put(SymbolValue, value)

	return input
}

func ratioInput(current float64, previous float64) nomagique.Frame {
	input := nomagique.Frame{}
	input.Put(SymbolCurrent, current)
	input.Put(SymbolPrevious, previous)

	return input
}

func scaleInput(value float64, scale float64) nomagique.Frame {
	input := nomagique.Frame{}
	input.Put(SymbolValue, value)
	input.Put(SymbolScale, scale)

	return input
}

func normalizedInput(value float64, baseline float64, ready float64) nomagique.Frame {
	input := nomagique.Frame{}
	input.Put(SymbolValue, value)
	input.Put(SymbolBaseline, baseline)
	input.Put(SymbolReady, ready)

	return input
}
