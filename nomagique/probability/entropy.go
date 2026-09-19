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
func NewEntropy(masses ...types.Float) Entropy {
	var acc float64

	return func(mass float64) float64 {
		m := mass
		if len(masses) > 0 && masses[0] != nil {
			m = masses[0](mass)
		}
		if m != 0 {
			acc += -m * math.Log(m)
		}

		return acc
	}
}
