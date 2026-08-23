package probability

import (
	"math"
	"testing"

	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/types"
)

func TestGeomean(t *testing.T) {
	input := types.Frame{}
	input.Put(nomagique.MustSampleSymbol(0), 4)
	input.Put(nomagique.MustSampleSymbol(1), 9)
	_, output, err := Geomean(types.Frame{}, input)

	if err != nil {
		t.Fatal(err)
	}

	if got := output.MustGet(SymbolResult); math.Abs(got-6) > 1e-12 {
		t.Fatalf("geomean=%v; want 6", got)
	}
}

func TestGeomeanRejectsInvalidEvidence(t *testing.T) {
	input := types.Frame{}
	input.Put(nomagique.MustSampleSymbol(0), math.Inf(1))

	if _, _, err := Geomean(types.Frame{}, input); err == nil {
		t.Fatal("non-finite evidence should fail")
	}
}
