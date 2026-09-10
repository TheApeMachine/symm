package calculus

import (
	"math"
	"testing"

	"github.com/theapemachine/symm/nomagique/tests"
)

func TestSqrtNext(t *testing.T) {
	tests.Check(
		t, tests.Case[float64, float64]{
			Name:      "sqrt",
			Seed:      0.0,
			Operation: NewSqrt[float64](),
			Reference: func(_, value float64) float64 {
				return math.Sqrt(value)
			},
		},
	)
}
