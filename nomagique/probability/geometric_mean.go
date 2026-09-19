package probability

import (
	"math"

	"github.com/theapemachine/symm/nomagique/types"
)

/*
NewGeometricMean owns exp(mean(log x)).
No structs, pure Value closure holding running geometric mean state.
*/
type GeometricMean types.Value[float64, float64]
func NewGeometricMean(values ...types.Float) GeometricMean {
	var count float64
	var sum float64

	return func(val float64) float64 {
		v := val
		if len(values) > 0 && values[0] != nil {
			v = values[0](val)
		}
		count++
		sum += math.Log(v)

		return math.Exp(sum / count)
	}
}
