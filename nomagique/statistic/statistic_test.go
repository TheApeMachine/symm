package statistic

import (
	"math"
	"testing"

	"github.com/theapemachine/symm/nomagique/types"
)

func TestMedianAndMaximum(t *testing.T) {
	input := sampleFrame(7, 1, 5, 3)
	_, median, err := Median(types.Frame{}, input)

	if err != nil {
		t.Fatal(err)
	}

	if got := median.MustGet(SymbolResult); got != 4 {
		t.Fatalf("median=%v; want 4", got)
	}

	_, maximum, err := Maximum(types.Frame{}, input)

	if err != nil {
		t.Fatal(err)
	}

	if got := maximum.MustGet(SymbolResult); got != 7 {
		t.Fatalf("maximum=%v; want 7", got)
	}
}

func TestMedianEmptyIsProvisional(t *testing.T) {
	_, output, err := Median(types.Frame{}, types.Frame{})

	if err != nil {
		t.Fatal(err)
	}

	if output.MustGet(SymbolReady) != 0 || output.MustGet(SymbolResult) != 0 {
		t.Fatal("empty median should be provisional")
	}
}

func TestMaxOf(t *testing.T) {
	symA := types.MustIntern("metric/a")
	symB := types.MustIntern("metric/b")
	symC := types.MustIntern("metric/c")

	maxPrimitive := MaxOf(symA, symB, symC)

	input := types.Frame{}
	input.Put(symA, 1.5)
	input.Put(symB, 4.2)
	input.Put(symC, -0.8)

	_, output, err := maxPrimitive(types.Frame{}, input)

	if err != nil {
		t.Fatal(err)
	}

	if got := output.MustGet(SymbolResult); got != 4.2 {
		t.Fatalf("MaxOf result=%v; want 4.2", got)
	}

	if got := output.MustGet(SymbolReady); got != 1 {
		t.Fatalf("MaxOf ready=%v; want 1", got)
	}

	// Empty input
	_, emptyOutput, err := maxPrimitive(types.Frame{}, types.Frame{})

	if err != nil {
		t.Fatal(err)
	}

	if got := emptyOutput.MustGet(SymbolReady); got != 0 {
		t.Fatalf("empty MaxOf ready=%v; want 0", got)
	}
}

func TestBranchingAndLikelihood(t *testing.T) {
	branchingInput := types.Frame{}
	branchingInput.Put(SymbolAlphaAA, 1)
	branchingInput.Put(SymbolAlphaAB, 0.5)
	branchingInput.Put(SymbolAlphaBA, 0.5)
	branchingInput.Put(SymbolAlphaBB, 1)
	branchingInput.Put(SymbolBeta, 2)
	_, branching, err := Branching(types.Frame{}, branchingInput)

	if err != nil {
		t.Fatal(err)
	}

	if got := branching.MustGet(SymbolSpectralRadius); math.Abs(got-0.75) > 1e-12 {
		t.Fatalf("spectral radius=%v; want 0.75", got)
	}

	likelihoodInput := types.Frame{}
	likelihoodInput.Put(SymbolLLHawkes, -120.5)
	likelihoodInput.Put(SymbolLLPoisson, -150)
	likelihoodInput.Put(SymbolLLSelf, -135)
	_, likelihood, err := Likelihood(types.Frame{}, likelihoodInput)

	if err != nil {
		t.Fatal(err)
	}

	if got := likelihood.MustGet(SymbolDeltaPoisson); got != 29.5 {
		t.Fatalf("Poisson delta=%v; want 29.5", got)
	}

	if got := likelihood.MustGet(SymbolDeltaSelf); got != 14.5 {
		t.Fatalf("self delta=%v; want 14.5", got)
	}
}

func sampleFrame(values ...float64) types.Frame {
	input := types.Frame{}

	for index, value := range values {
		input.Put(types.MustSampleSymbol(index), value)
	}

	return input
}
