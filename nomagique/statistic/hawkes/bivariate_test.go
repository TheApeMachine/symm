package hawkes

import (
	"math"
	"testing"

	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestBivariatePrimitiveDelivery(t *testing.T) {
	graph := NewBivariate()
	observe := func(key string, at int64, mark float64) *data.Measurement[float64] {
		t.Helper()
		result, err := transport.Evaluate(graph, transport.Values(Event{Key: key, At: at, Mark: mark}))
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	a := observe("A", 1_000_000_000, 1)
	b := observe("B", 1_000_000_000, -1)
	if a.Err != nil || b.Err != nil {
		t.Fatalf("first events: %v %v", a.Err, b.Err)
	}
	if len(graph.states["A"].samples) != 1 || len(graph.states["B"].samples) != 1 {
		t.Fatal("key isolation lost")
	}
	for _, mark := range []float64{0, math.NaN(), math.Inf(1)} {
		if observe("A", 2_000_000_000, mark).Err == nil {
			t.Fatal("invalid mark admitted")
		}
	}
	if observe("A", 0, 1).Err == nil {
		t.Fatal("regressing event admitted")
	}
	if len(graph.states["A"].samples) != 1 {
		t.Fatal("rejected event mutated arrival history")
	}
	if observe("A", 2_000_000_000, -1).Err != nil {
		t.Fatal("rejection poisoned next valid run")
	}
	if len(graph.states["A"].samples) != 2 {
		t.Fatal("second delivery was lost")
	}
}
