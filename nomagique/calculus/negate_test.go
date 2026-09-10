package calculus

import (
	"testing"

	"github.com/theapemachine/symm/nomagique/tests"
)

func TestNegateNext(t *testing.T) {
	tests.Check(
		t, tests.Case[float64, float64]{
			Name:      "negate",
			Seed:      0.0,
			Operation: NewNegate[float64](),
			Reference: func(_, value float64) float64 {
				return -value
			},
		},
	)
}
