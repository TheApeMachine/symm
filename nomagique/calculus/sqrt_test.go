package calculus

import (
	"math"
	"testing"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestSqrtNext(t *testing.T) {
	tests.Check(
		t, tests.Case[float64, float64]{
			Name: "sqrt",
			Seed: 0.0,
			Factory: func() core.Primitive {
				return NewSqrt()
			},
			Reference: func(_ float64, value float64) float64 {
				return math.Sqrt(value)
			},
		},
	)
}
