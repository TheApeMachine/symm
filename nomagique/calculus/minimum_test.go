package calculus

import (
	"math"
	"testing"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestMinimumNext(t *testing.T) {
	tests.Check(
		t, tests.Case[float64, float64]{
			Name: "minimum",
			Seed: 100.0,
			Factory: func() core.Primitive {
				return NewMinimum(100.0)
			},
			Reference: func(current, value float64) float64 {
				return math.Min(current, value)
			},
		},
	)
}
