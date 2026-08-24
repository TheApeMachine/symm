package probability

import (
	"math"
	"testing"

	"github.com/theapemachine/symm/nomagique/types"
)

func TestGeomean(t *testing.T) {
	input := types.Frame{}
	input.Put(types.MustSampleSymbol(0), 4)
	input.Put(types.MustSampleSymbol(1), 9)
	output := Geomean(input)

	if output.Err != nil {
		t.Fatal(output.Err)
	}

	if got := output.MustGet(SymbolResult); math.Abs(got-6) > 1e-12 {
		t.Fatalf("geomean=%v; want 6", got)
	}
}

func TestGeomeanRejectsInvalidEvidence(t *testing.T) {
	input := types.Frame{}
	input.Put(types.MustSampleSymbol(0), math.Inf(1))

	if output := Geomean(input); output.Err == nil {
		t.Fatal("non-finite evidence should fail")
	}
}
