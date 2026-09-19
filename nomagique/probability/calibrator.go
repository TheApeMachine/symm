package probability

import (
	"slices"

	"github.com/theapemachine/symm/nomagique/types"
)

/*
NewCalibrator creates a rank calibrator against retained prior errors.
Retention is a configured Value transform (e.g. sequence.NewTail or nil for all history).
Returns the empirical rank (0.0 to 1.0) of the incoming value.
If there are no priors, returns 0.5.
*/
type Calibrator types.Value[float64, float64]
func NewCalibrator(retention types.Value[[]float64, []float64]) Calibrator {
	var history []float64

	return func(val float64) float64 {
		rank := 0.5

		if len(history) > 0 {
			hits := 0.0
			for _, prior := range history {
				if prior > val {
					hits++
				}
			}
			rank = hits / float64(len(history))
		}

		if retention != nil {
			candidate := append(slices.Clone(history), val)
			history = retention(candidate)
		} else {
			history = append(history, val)
		}

		return rank
	}
}
