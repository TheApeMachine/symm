package learning_test

import (
	"math"
	"testing"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/learning"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestTargetsNext(t *testing.T) {
	for _, test := range []struct {
		name string
		node core.Primitive
	}{
		{"delta", learning.NewDeltaTarget()},
		{"identity", learning.NewIdentityTarget()},
		{"ratio", learning.NewRatioTarget()},
		{"binary", learning.NewBinaryTarget()},
		{"directional", learning.NewDirectionalTarget(store.NewConstant(core.From(0.5)))},
	} {
		t.Run(test.name, func(t *testing.T) { tests.CheckTarget(t, test.name, test.node, 0.5) })
	}
}

func TestTargetInvalidInput(t *testing.T) {
	for _, poison := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		node := learning.NewDeltaTarget()
		_ = tests.Drain(t, node, tests.Values(tests.Record(map[string]any{"current": poison, "past": 1.0})))
		if node.Error() == nil {
			t.Fatal("nonfinite input accepted")
		}
	}
	node := learning.NewRatioTarget()
	_ = tests.Drain(t, node, tests.Values(tests.Record(map[string]any{"current": 1.0, "past": 0.0})))
	if node.Error() == nil {
		t.Fatal("zero divisor accepted")
	}
}

func TestTargetConfiguredConnection(t *testing.T) {
	band := store.NewRetained(core.From(0.5))
	node := learning.NewDirectionalTarget(transport.NewApply(band, nil))
	input := tests.Record(map[string]any{"current": 2.0, "past": 1.0})
	out := tests.Drain(t, node, tests.Values(input))
	tests.Sound(t, node)
	tests.EqualNumber(t, out[0], 1)
	tests.Drain(t, band, tests.Values(2.0))
	out = tests.Drain(t, node, tests.Values(input))
	tests.Sound(t, node)
	tests.EqualNumber(t, out[0], 0)
	// Mapping a run changes only topology, not the target implementation.
	out = tests.Drain(t, transport.NewMap(node), tests.Values(input, input))
	if len(out) != 2 {
		t.Fatal("mapped target dropped input")
	}
}

func TestDirectionalTargetInvalidConfiguration(t *testing.T) {
	for _, band := range []float64{-1, math.NaN(), math.Inf(1)} {
		node := learning.NewDirectionalTarget(store.NewConstant(core.From(band)))
		tests.Drain(t, node, tests.Values(tests.Record(map[string]any{"current": 2.0, "past": 1.0})))
		if node.Error() == nil {
			t.Fatal("invalid deadband accepted")
		}
	}
}
