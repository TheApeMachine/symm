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
func NewGeomean(values ...types.Float) Geomean {
	return Geomean(NewGeometricMean(values...))
}

/*
NewShannonAmbiguity folds a run of non-negative weights into their running normalized Shannon entropy.
No structs, pure Value closure holding running values and total.
*/
type ShannonAmbiguity types.Value[float64, float64]
func NewShannonAmbiguity(values ...types.Float) ShannonAmbiguity {
	var vals []float64
	var total float64

	return func(val float64) float64 {
		v := val
		if len(values) > 0 && values[0] != nil {
			v = values[0](val)
		}
		vals = append(vals, v)
		total += v

		if len(vals) < 2 || total == 0 {
			return 0
		}

		entropy := 0.0
		for _, item := range vals {
			p := item / total
			if p > 0 {
				entropy -= p * math.Log(p)
			}
		}

		return entropy / math.Log(float64(len(vals)))
	}
}
