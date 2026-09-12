package arithmetic

import (
	"testing"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestSubtractNext(t *testing.T) {
	tests.Check(
		t, tests.Case[float64, float64]{
			Name: "subtract",
			Seed: 0.0,
			Factory: func() core.Primitive {
				return NewSubtract(0.0)
			},
			Reference: func(acc, val float64) float64 {
				return acc - val
			},
		},
	)
}
