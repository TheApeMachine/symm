package transport_test

import (
	"testing"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestBatchNext(t *testing.T) {
	seen := 0
	source := transport.NewGenerator(func(yield func(float64) bool) {
		for _, value := range []float64{1, 2, 3, 4, 5} {
			seen++
			if !yield(value) {
				return
			}
		}
	})
	batch := transport.NewBatch(source, transport.NewIO(core.From(2)))
	for _, want := range [][]float64{{1, 2}, {3, 4}, {5}} {
		values := tests.Drain(t, transport.NewPipe(), batch)
		if len(values) != len(want) {
			t.Fatalf("batch: %v, wanted %v", values, want)
		}
		for i, value := range values {
			tests.EqualNumber(t, value, want[i])
		}
	}
	if seen != 5 {
		t.Fatalf("source advanced %d times", seen)
	}
	if values := tests.Drain(t, transport.NewPipe(), batch); len(values) != 0 {
		t.Fatal("source restarted")
	}
}
