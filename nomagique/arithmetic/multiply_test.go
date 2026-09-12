package arithmetic

import (
	"testing"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestMultiplyNext(t *testing.T) {
	tests.Check(
		t, tests.Case[float64, float64]{
			Name: "multiply",
			Seed: 1.0,
			Factory: func() core.Primitive {
				return NewMultiply(1.0)
			},
			Reference: func(acc, val float64) float64 {
				return acc * val
			},
		},
	)
}
