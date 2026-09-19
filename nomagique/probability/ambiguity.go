package probability

import (
	"math"

	"github.com/theapemachine/symm/nomagique/types"
)

/*
NewAmbiguity divides entropy by the entropy of an equal-mass distribution.
A one-member distribution has zero ambiguity by definition.
No structs, pure Value closure.
*/
type Ambiguity types.Value[[]float64, float64]
func NewAmbiguity() Ambiguity {
	return func(values []float64) float64 {
		if len(values) <= 1 {
			return 0
		}

		var total float64
		for _, val := range values {
			total += val
		}

		if total == 0 {
			return 0
		}

		entropy := 0.0
		for _, val := range values {
			p := val / total
			if p > 0 {
				entropy -= p * math.Log(p)
			}
		}

		return entropy / math.Log(float64(len(values)))
	}
}
