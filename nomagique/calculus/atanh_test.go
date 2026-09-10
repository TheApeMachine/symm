package calculus

import (
	"math"
	"testing"

	"github.com/theapemachine/symm/nomagique/tests"
)

func TestAtanhNext(t *testing.T) {
	tests.Check(
		t, tests.Case[float64, float64]{
			Name:      "atanh",
			Seed:      0.0,
			Operation: NewAtanh[float64](),
			Reference: func(_, value float64) float64 {
				return math.Atanh(value)
			},
			CustomVectors: [][]float64{
				{0.5},
				{-0.5, 0.25, 0.0},
			},
		},
	)
}
