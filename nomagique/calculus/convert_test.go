package calculus

import (
	"testing"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestConvertNext(t *testing.T) {
	tests.Check(
		t, tests.Case[float64, float64]{
			Name: "convert",
			Seed: 0.0,
			Factory: func() core.Primitive {
				return NewConvert()
			},
			Reference: func(_ float64, value float64) float64 {
				return value
			},
		},
	)
}
