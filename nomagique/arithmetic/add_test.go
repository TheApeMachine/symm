package arithmetic

import (
	"testing"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestAddNext(t *testing.T) {
	tests.Check(
		t, tests.Case[float64, float64]{
			Name: "add",
			Seed: 0.0,
			Factory: func() core.Primitive {
				return NewAdd(0.0)
			},
			Reference: func(acc, val float64) float64 {
				return acc + val
			},
		},
	)
}
