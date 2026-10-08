package calculus

import (
	"math"
	"testing"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestAtanhNext(t *testing.T) {
	tests.Check(
		t, tests.Case[float64, float64]{
			Name: "atanh",
			Seed: 0.0,
			Factory: func() core.Primitive {
				return NewAtanh()
			},
			Reference: func(_ float64, value float64) float64 {
				return math.Atanh(value)
			},
			CustomVectors: [][]float64{
				{0.0},
				{0.5, -0.5},
				{0.9, -0.9},
			},
		},
	)
}
