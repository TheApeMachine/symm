package temporal_test

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/temporal"
	"github.com/theapemachine/symm/nomagique/tests"
	"testing"
)

func TestGovernorNext(t *testing.T) {
	node := temporal.NewGovernor(store.NewConstant(tests.Record(map[string]any{"capacity": 2.0})), equation.NewMean())
	outputs := tests.Drain(t, node, tests.Values(1.0, 3.0, 7.0, 9.0))
	tests.Sound(t, node)
	for index, expected := range []float64{0, 2, 5, 8} {
		tests.EqualNumber(t, outputs[index], expected)
	}
	_ = core.Primitive(node)
}
