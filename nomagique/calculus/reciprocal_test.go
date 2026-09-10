package calculus

import (
	"testing"

	"github.com/theapemachine/symm/nomagique/tests"
)

func TestReciprocalNext(t *testing.T) {
	tests.Check(
		t, tests.Case[float64, float64]{
			Name:      "reciprocal",
			Seed:      0.0,
			Operation: NewReciprocal[float64](),
			Reference: func(_, value float64) float64 {
				return 1 / value
			},
		},
	)
}
