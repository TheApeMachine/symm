package linear_test

import (
	"github.com/theapemachine/symm/nomagique/equation/linear"
	"github.com/theapemachine/symm/nomagique/tests"
	"testing"
)

func TestSpectralRadius2Next(t *testing.T) {
	node := linear.NewSpectralRadius2()
	for _, c := range []struct{ a, b, c, d, r float64 }{{0, -1, 1, 0, 1}, {-3, 0, 0, 2, 3}, {1, 0, 0, 1, 1}} {
		out := tests.Drain(t, node, tests.Values(tests.Record(map[string]any{"a": c.a, "b": c.b, "c": c.c, "d": c.d})))
		tests.Sound(t, node)
		tests.EqualNumber(t, out[0], c.r)
	}
}
