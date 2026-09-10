package calculus

import (
	"math"
	"testing"

	"github.com/theapemachine/symm/nomagique/tests"
)

func TestSignNext(t *testing.T) {
	tests.Check(
		t, tests.Case[float64, float64]{
			Name:      "sign",
			Seed:      0.0,
			Operation: NewSign[float64](),
			Reference: func(_, value float64) float64 {
				if value == 0 {
					return value
				}

				return math.Copysign(1, value)
			},
		},
	)
}
