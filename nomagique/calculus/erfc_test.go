package calculus

import (
	"math"
	"testing"

	"github.com/theapemachine/symm/nomagique/tests"
)

func TestErfcNext(t *testing.T) {
	tests.Check(
		t, tests.Case[float64, float64]{
			Name:      "erfc",
			Seed:      0.0,
			Operation: NewErfc[float64](),
			Reference: func(_, value float64) float64 {
				return math.Erfc(value)
			},
		},
	)
}
