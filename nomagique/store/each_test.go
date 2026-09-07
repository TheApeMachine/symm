package store_test

import (
	"github.com/theapemachine/symm/nomagique/arithmetic"
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
	"math"
	"testing"
)

// The old Each handed inert carriers directly to Next. Map owns delivery now;
// replacement by a constant is explicit, not a special case in core.From.
func TestEachComposition(t *testing.T) {
	cases := []struct {
		name      string
		values    []float64
		operation core.Primitive
		want      float64
	}{
		{"squares", []float64{1, 2, 3}, calculus.NewSquare(transport.NewIO(core.From(0.0))), 14},
		{"count", []float64{4, 7, 9}, store.NewConstant(core.From(1.0)), 3},
		{"empty", nil, store.NewConstant(core.From(1.0)), 0},
		{"reciprocals", []float64{1, 2}, calculus.NewReciprocal(transport.NewIO(core.From(0.0))), 1.5},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			graph := transport.NewPipe(transport.NewSpread[float64](), transport.NewMap(test.operation), arithmetic.NewAdd[float64](transport.NewIO(core.From(0.0))))
			for range 2 {
				actual, err := transport.Evaluate[float64](graph, core.From(test.values))
				if err != nil || math.Abs(actual-test.want) > 1e-12 {
					t.Fatalf("got %v, %v; want %v", actual, err, test.want)
				}
			}
		})
	}
	mean := transport.NewPipe(transport.NewSpread[float64](), equation.NewMean())
	actual, err := transport.Evaluate[float64](mean, core.From([]float64{4, 7, 9, 8}))
	if err != nil || actual != 7 {
		t.Fatalf("mean %v, %v", actual, err)
	}
}
