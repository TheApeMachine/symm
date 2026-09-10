package arithmetic

import (
	"testing"

	"github.com/theapemachine/symm/nomagique/tests"
)

func TestMultiplyNext(t *testing.T) {
	tests.Check(
		t, tests.Case[float64, float64]{
			Name:      "multiply",
			Seed:      0.0,
			Operation: NewMultiply(0.0),
			Reference: func(acc, val float64) float64 {
				return acc * val
			},
		},
	)
}
