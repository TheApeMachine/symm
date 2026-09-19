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
func NewGeometricMean() GeometricMean {
	var count float64
	var sum float64

	return func(val float64) float64 {
		count++
		sum += math.Log(val)

		return math.Exp(sum / count)
	}
}
