package calculus

import (
	"math"
	"testing"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestFloorNext(t *testing.T) {
	tests.Check(
		t, tests.Case[float64, float64]{
			Name: "floor",
			Seed: 0.0,
			Factory: func() core.Primitive {
				return NewFloor()
			},
			Reference: func(_ float64, value float64) float64 {
				return math.Floor(value)
			},
		},
	)
}
