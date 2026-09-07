package store_test

import (
	"github.com/theapemachine/symm/nomagique/arithmetic"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
	"testing"
)

func TestSpreadComposition(t *testing.T) {
	sum := transport.NewPipe(transport.NewSpread[float64](), arithmetic.NewAdd[float64](transport.NewIO(core.From(0.0))))
	for _, test := range []struct {
		values []float64
		want   float64
	}{{[]float64{1, 2, 3, 4}, 10}, {[]float64{1, 2, 3}, 6}, {nil, 0}, {[]float64{1, 2, 3}, 6}} {
		value, err := transport.Evaluate[float64](sum, core.From(test.values))
		if err != nil || value != test.want {
			t.Fatalf("sum %v, %v; want %v", value, err, test.want)
		}
	}
	product := transport.NewPipe(transport.NewSpread[float64](), arithmetic.NewMultiply[float64](transport.NewIO(core.From(1.0))))
	value, err := transport.Evaluate[float64](product, core.From([]float64{1, 2, 3}))
	if err != nil || value != 6 {
		t.Fatalf("product %v %v", value, err)
	}
	// Cross-run accumulation must be owned by Retained, not a hidden Add mode.
	retained := store.NewRetained(core.From(0.0))
	accumulating := transport.NewPipe(transport.NewSpread[float64](), arithmetic.NewAdd[float64](retained), retained)
	for _, want := range []float64{6, 12} {
		value, err := transport.Evaluate[float64](accumulating, core.From([]float64{1, 2, 3}))
		if err != nil || value != want {
			t.Fatalf("retained %v %v; want %v", value, err, want)
		}
	}
	input := transport.NewIO(core.From([]float64{1, 2}), core.From([]float64{3}))
	output := sum.Next(input)
	if core.To[float64](output) != 6 {
		t.Fatal("multiple collections not flattened")
	}
	if sum.Next(input) != nil {
		t.Fatal("run not closed")
	}
}
