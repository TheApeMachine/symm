package transport_test

import (
	"errors"
	"testing"

	"github.com/theapemachine/symm/nomagique/arithmetic"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestEvaluate(t *testing.T) {
	memory := store.NewRetained(core.From(0.0))
	graph := transport.NewPipe(arithmetic.NewAdd[float64](memory), memory)
	for index, expected := range []float64{1, 3, 6, 10} {
		actual, err := transport.Evaluate[float64](graph, core.From(float64(index+1)))
		if err != nil || actual != expected {
			t.Fatalf("event %d: %g, %v; want %g", index, actual, err, expected)
		}
	}
}

func TestEvaluateRejectsWrongShape(t *testing.T) {
	for _, graph := range []core.Primitive{nil, transport.NewIO(), transport.NewIO(core.From(1.0), core.From(2.0))} {
		if _, err := transport.Evaluate[float64](graph, core.From(0.0)); err == nil {
			t.Fatal("missing cardinality error")
		}
	}
	if _, err := transport.Evaluate[float64](transport.NewPipe(), core.From("not a number")); err == nil {
		t.Fatal("missing conversion error")
	}
}

type finalFailure struct {
	core.PrimitiveError
	delivered bool
}

func (source *finalFailure) Next(core.Primitive) core.Primitive {
	if !source.delivered {
		source.delivered = true
		return core.From(1.0)
	}
	source.Error(errors.New("failed on final nil"))
	return nil
}
func (*finalFailure) Read() any { return nil }

func TestEvaluatePropagatesFinalFailure(t *testing.T) {
	if _, err := transport.Evaluate[float64](&finalFailure{}, core.From(0.0)); err == nil {
		t.Fatal("final nil error was lost")
	}
}
