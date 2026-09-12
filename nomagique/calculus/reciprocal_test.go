package calculus

import (
	"testing"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestReciprocalNext(t *testing.T) {
	tests.Check(
		t, tests.Case[float64, float64]{
			Name: "reciprocal",
			Seed: 0.0,
			Factory: func() core.Primitive {
				return NewReciprocal()
			},
			Reference: func(_ float64, value float64) float64 {
				return 1.0 / value
			},
		},
	)
}
