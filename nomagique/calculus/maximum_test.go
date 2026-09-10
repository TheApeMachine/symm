package calculus

import (
	"testing"

	"github.com/theapemachine/symm/nomagique/tests"
)

func TestMaximumNext(t *testing.T) {
	tests.Check(
		t, tests.Case[float64, float64]{
			Name:      "maximum",
			Seed:      -100.0,
			Operation: NewMaximum(-100.0),
			Reference: func(held, value float64) float64 {
				if value > held {
					return value
				}

				return held
			},
		},
	)
}
