package calculus

import (
	"testing"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestNegateNext(t *testing.T) {
	tests.Check(
		t, tests.Case[float64, float64]{
			Name: "negate",
			Seed: 0.0,
			Factory: func() core.Primitive {
				return NewNegate()
			},
			Reference: func(_ float64, value float64) float64 {
				return -value
			},
		},
	)
}
