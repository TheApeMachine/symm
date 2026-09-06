package equation_test

import (
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/tests"
	"math"
	"testing"
)

func TestNewSigmoid(t *testing.T) {
	node := equation.NewSigmoid()
	for _, value := range []float64{0, 10, -10} {
		out := tests.Drain(t, node, tests.Values(value))
		tests.Sound(t, node)
		if len(out) != 1 {
			t.Fatal("expected one sigmoid result")
		}
		tests.EqualNumber(t, out[0], 1/(1+math.Exp(-value)))
	}
}
