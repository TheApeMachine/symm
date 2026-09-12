package calculus

import (
	"math"
	"testing"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestLogNext(t *testing.T) {
	tests.Check(
		t, tests.Case[float64, float64]{
			Name: "log",
			Seed: 0.0,
			Factory: func() core.Primitive {
				return NewLog()
			},
			Reference: func(_ float64, value float64) float64 {
				return math.Log(value)
			},
		},
	)
}
