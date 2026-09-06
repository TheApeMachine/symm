package vector_test

import (
	"testing"

	"github.com/theapemachine/symm/nomagique/algo"
	"github.com/theapemachine/symm/nomagique/arithmetic"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
	"github.com/theapemachine/symm/nomagique/vector"
)

func TestApplyNext(t *testing.T) {
	node := vector.NewApply(transport.NewIO(
		arithmetic.NewAdd[float64](tests.Values(2.0)),
		arithmetic.NewMultiply[float64](tests.Values(3.0)),
	))
	for _, values := range [][]float64{{4, 5}, {-2, 7}} {
		out := tests.Drain(t, node, tests.Values(values...))
		tests.Sound(t, node)
		if len(out) != 2 {
			t.Fatalf("got %d values", len(out))
		}
		tests.EqualNumber(t, out[0], values[0]+2)
		tests.EqualNumber(t, out[1], values[1]*3)
	}
}

func TestApplyIndependentState(t *testing.T) {
	node := vector.NewApply(transport.NewIO(algo.NewWelford(), algo.NewWelford()))
	for index, pair := range [][2]float64{{2, 20}, {4, 40}, {6, 60}} {
		out := tests.Drain(t, node, tests.Values(pair[:]...))
		tests.Sound(t, node)
		if len(out) != 2 {
			t.Fatal("coordinate missing")
		}
		left, right := tests.Fields(t, out[0]), tests.Fields(t, out[1])
		tests.EqualNumber(t, tests.Number(t, left, "count"), float64(index+1))
		tests.EqualNumber(t, tests.Number(t, right, "count"), float64(index+1))
		tests.EqualNumber(t, tests.Number(t, left, "mean"), float64(index+2))
		tests.EqualNumber(t, tests.Number(t, right, "mean"), 10*float64(index+2))
	}
}

func TestApplyPreservesEndpointArity(t *testing.T) {
	endpoint := transport.NewFan(transport.NewPipe(), transport.NewIO(
		store.NewConstant(core.From("first")), store.NewConstant(core.From("second")),
	))
	node := vector.NewApply(transport.NewIO(endpoint))
	out := tests.Drain(t, node, tests.Values(7.0))
	tests.Sound(t, node)
	if len(out) != 2 || out[0] != "first" || out[1] != "second" {
		t.Fatalf("lost endpoint arity: %v", out)
	}
}

func TestApplyShape(t *testing.T) {
	node := vector.NewApply(transport.NewIO(transport.NewPipe()))
	_ = tests.Drain(t, node, tests.Values(1.0, 2.0))
	if node.Error() == nil {
		t.Fatal("different endpoint cardinalities accepted")
	}
}
