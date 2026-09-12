package arithmetic

import (
	"testing"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestDivideNext(t *testing.T) {
	tests.Check(
		t, tests.Case[float64, float64]{
			Name: "divide",
			Seed: 1024.0,
			Factory: func() core.Primitive {
				return NewDivide(1024.0)
			},
			Reference: func(acc, val float64) float64 {
				return acc / val
			},
		},
	)
}
