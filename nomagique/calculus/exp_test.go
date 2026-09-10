package calculus

import (
	"math"
	"testing"

	"github.com/theapemachine/symm/nomagique/tests"
)

func TestExpNext(t *testing.T) {
	tests.Check(
		t, tests.Case[float64, float64]{
			Name:      "exp",
			Seed:      0.0,
			Operation: NewExp[float64](),
			Reference: func(_, value float64) float64 {
				return math.Exp(value)
			},
		},
	)
}
