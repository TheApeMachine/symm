package collection_test

import (
	"github.com/theapemachine/symm/nomagique/collection"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
	"slices"
	"testing"
)

func TestOrderNext(t *testing.T) {
	input := []float64{3, 1, 2}
	out := tests.Drain(t, collection.NewOrder[float64](), transport.NewIO(core.From(input)))
	if !slices.Equal(out[0].([]float64), []float64{1, 2, 3}) {
		t.Fatal(out)
	}
	if !slices.Equal(input, []float64{3, 1, 2}) {
		t.Fatal("mutated input")
	}
}
