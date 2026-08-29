package probability

import (
	"math"
	"testing"

	"github.com/theapemachine/symm/nomagique/types"
)

func TestGeomean(t *testing.T) {
	output := types.Frame{}
	output.Put(types.MustSampleSymbol(0), 4)
	output.Put(types.MustSampleSymbol(1), 9)
	Geomean(&output)

	if output.Err != nil {
		t.Fatal(output.Err)
	}

	if got := output.MustGet(SymbolResult); math.Abs(got-6) > 1e-12 {
		t.Fatalf("geomean=%v; want 6", got)
	}
}

func TestGeomeanRejectsInvalidEvidence(t *testing.T) {
	output := types.Frame{}
	output.Put(types.MustSampleSymbol(0), math.Inf(1))

	Geomean(&output)
	if output.Err == nil {
		t.Fatal("non-finite evidence should fail")
	}
}
