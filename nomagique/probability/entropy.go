package probability

import (
	"math"

	"github.com/theapemachine/symm/nomagique/types"
)

/*
NewEntropy owns -sum(p log p). Zero mass contributes its limiting value zero.
No structs, pure Value closure holding running accumulation state.
*/
type Entropy types.Value[float64, float64]
func NewEntropy() Entropy {
	var acc float64

	return func(mass float64) float64 {
		if mass != 0 {
			acc += -mass * math.Log(mass)
		}

		return acc
	}
}
