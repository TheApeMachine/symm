package calculus

import (
	"testing"

	"github.com/theapemachine/symm/nomagique/tests"
)

func TestSquareNext(t *testing.T) {
	tests.Check(
		t, tests.Case[float64, float64]{
			Name:      "square",
			Seed:      0.0,
			Operation: NewSquare[float64](),
			Reference: func(_, value float64) float64 {
				return value * value
			},
		},
	)
}
