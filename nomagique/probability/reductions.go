package probability

import (
	"math"

	"github.com/theapemachine/symm/nomagique/types"
)

/*
NewGeomean folds a run of values into their running geometric mean, exp(mean(log x)).
No structs, pure Value closure.
*/
type Geomean types.Value[float64, float64]
func NewGeomean() Geomean {
	return Geomean(NewGeometricMean())
}

/*
NewShannonAmbiguity folds a run of non-negative weights into their running normalized Shannon entropy.
No structs, pure Value closure holding running values and total.
*/
type ShannonAmbiguity types.Value[float64, float64]
func NewShannonAmbiguity() ShannonAmbiguity {
	var values []float64
	var total float64

	return func(val float64) float64 {
		values = append(values, val)
		total += val

		if len(values) < 2 || total == 0 {
			return 0
		}

		entropy := 0.0
		for _, v := range values {
			p := v / total
			if p > 0 {
				entropy -= p * math.Log(p)
			}
		}

		return entropy / math.Log(float64(len(values)))
	}
}
