package statistic

import (
	"math"
	"testing"

	"github.com/theapemachine/symm/nomagique"
)

func TestMedianAndMaximum(t *testing.T) {
	input := sampleFrame(7, 1, 5, 3)
	_, median, err := Median(nomagique.Frame{}, input)

	if err != nil {
		t.Fatal(err)
	}

	if got := median.MustGet(SymbolResult); got != 4 {
		t.Fatalf("median=%v; want 4", got)
	}

	_, maximum, err := Maximum(nomagique.Frame{}, input)

	if err != nil {
		t.Fatal(err)
	}

	if got := maximum.MustGet(SymbolResult); got != 7 {
		t.Fatalf("maximum=%v; want 7", got)
	}
}

func TestMedianEmptyIsProvisional(t *testing.T) {
	_, output, err := Median(nomagique.Frame{}, nomagique.Frame{})

	if err != nil {
		t.Fatal(err)
	}

	if output.MustGet(SymbolReady) != 0 || output.MustGet(SymbolResult) != 0 {
		t.Fatal("empty median should be provisional")
	}
}

func TestBranchingAndLikelihood(t *testing.T) {
	branchingInput := nomagique.Frame{}
	branchingInput.Put(SymbolAlphaAA, 1)
	branchingInput.Put(SymbolAlphaAB, 0.5)
	branchingInput.Put(SymbolAlphaBA, 0.5)
	branchingInput.Put(SymbolAlphaBB, 1)
	branchingInput.Put(SymbolBeta, 2)
	_, branching, err := Branching(nomagique.Frame{}, branchingInput)

	if err != nil {
		t.Fatal(err)
	}

	if got := branching.MustGet(SymbolSpectralRadius); math.Abs(got-0.75) > 1e-12 {
		t.Fatalf("spectral radius=%v; want 0.75", got)
	}

	likelihoodInput := nomagique.Frame{}
	likelihoodInput.Put(SymbolLLHawkes, -120.5)
	likelihoodInput.Put(SymbolLLPoisson, -150)
	likelihoodInput.Put(SymbolLLSelf, -135)
	_, likelihood, err := Likelihood(nomagique.Frame{}, likelihoodInput)

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

func sampleFrame(values ...float64) nomagique.Frame {
	input := nomagique.Frame{}

	for index, value := range values {
		input.Put(nomagique.MustSampleSymbol(index), value)
	}

	return input
}
