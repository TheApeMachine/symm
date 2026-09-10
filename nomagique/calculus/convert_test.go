package calculus

import (
	"testing"

	"github.com/theapemachine/symm/nomagique/tests"
)

func TestConvertNext(t *testing.T) {
	tests.Check(
		t, tests.Case[int64, float64]{
			Name:      "convert",
			Seed:      0.0,
			Operation: NewConvert[int64, float64](),
			Reference: func(_ float64, value int64) float64 {
				return float64(value)
			},
		},
	)
}
