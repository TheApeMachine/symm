package calculus

import (
	"math"
	"testing"

	"github.com/theapemachine/symm/nomagique/tests"
)

func TestAbsoluteNext(t *testing.T) {
	tests.Check(
		t, tests.Case[float64, float64]{
			Name:      "absolute",
			Seed:      0.0,
			Operation: NewAbsolute[float64](),
			Reference: func(_, value float64) float64 {
				return math.Abs(value)
			},
		},
	)
}
