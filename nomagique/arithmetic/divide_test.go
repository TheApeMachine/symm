package arithmetic

import (
	"testing"

	"github.com/theapemachine/symm/nomagique/tests"
)

func TestDivideNext(t *testing.T) {
	tests.Check(
		t, tests.Case[float64, float64]{
			Name:      "divide",
			Seed:      1024.0,
			Operation: NewDivide(1024.0),
			Reference: func(acc, val float64) float64 {
				return acc / val
			},
		},
	)
}
