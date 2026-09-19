package probability

import (
	"github.com/theapemachine/symm/nomagique/types"
)

/*
NewNormalize divides each arrival by the total.
No structs, pure Value closure.
*/
type Normalize types.Value[[]float64, []float64]

func NewNormalize(values ...types.Float) Normalize {
	return func(in []float64) []float64 {
		vals := in
		if len(values) > 0 {
			vals = make([]float64, len(values))
			for i, v := range values {
				if v != nil {
					vals[i] = v(in)
				}
			}
		}

		var total float64
		for _, val := range vals {
			total += val
		}

		if total == 0 {
			return nil
		}

		out := make([]float64, len(vals))
		for index, val := range vals {
			out[index] = val / total
		}

		return out
	}
}
