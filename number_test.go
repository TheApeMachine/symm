package nomagique_test

import (
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/arithmetic"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
	"testing"
)

func TestNumberIsPrimitive(t *testing.T) {
	graph := nomagique.Number(arithmetic.NewAdd[float64](transport.NewIO(core.From(2.0))))
	outer := arithmetic.NewMultiply[float64](transport.NewIO(core.From(3.0)))
	got := tests.Drain(t, outer, transport.NewApply(graph, transport.NewIO(core.From(5.0))))
	tests.EqualNumber(t, got[0], 21)
}
