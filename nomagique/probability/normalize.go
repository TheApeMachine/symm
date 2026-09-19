package probability

import (
	"github.com/theapemachine/symm/nomagique/types"
)

/*
NewNormalize divides each arrival by the total.
No structs, pure Value closure.
*/
type Normalize types.Value[[]float64, []float64]
func NewNormalize() Normalize {
	return func(values []float64) []float64 {
		var total float64
		for _, val := range values {
			total += val
		}

		if total == 0 {
			return nil
		}

		out := make([]float64, len(values))
		for index, val := range values {
			out[index] = val / total
		}

		return out
	}
}
