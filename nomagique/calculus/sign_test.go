package calculus

import (
	"math"
	"testing"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestSignNext(t *testing.T) {
	tests.Check(
		t, tests.Case[float64, float64]{
			Name: "sign",
			Seed: 0.0,
			Factory: func() core.Primitive {
				return NewSign()
			},
			Reference: func(_ float64, value float64) float64 {
				if value != 0 {
					return math.Copysign(1, value)
				}
				return value
			},
		},
	)
}
