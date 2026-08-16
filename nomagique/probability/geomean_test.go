package probability

import (
	"math"
	"testing"

	"github.com/theapemachine/symm/nomagique"
)

func TestGeomean(t *testing.T) {
	input := nomagique.Frame{}
	input.Put(nomagique.MustSampleSymbol(0), 4)
	input.Put(nomagique.MustSampleSymbol(1), 9)
	_, output, err := Geomean(nomagique.Frame{}, input)

	if err != nil {
		t.Fatal(err)
	}

	if got := output.MustGet(SymbolResult); math.Abs(got-6) > 1e-12 {
		t.Fatalf("geomean=%v; want 6", got)
	}
}

func TestGeomeanRejectsInvalidEvidence(t *testing.T) {
	input := nomagique.Frame{}
	input.Put(nomagique.MustSampleSymbol(0), math.Inf(1))

	if _, _, err := Geomean(nomagique.Frame{}, input); err == nil {
		t.Fatal("non-finite evidence should fail")
	}
}
