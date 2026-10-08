package calculus

import (
	"math"
	"testing"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestMaximumNext(t *testing.T) {
	tests.Check(
		t, tests.Case[float64, float64]{
			Name: "maximum",
			Seed: -100.0,
			Factory: func() core.Primitive {
				return NewMaximum(-100.0)
			},
			Reference: func(current, value float64) float64 {
				return math.Max(current, value)
			},
		},
	)
}
