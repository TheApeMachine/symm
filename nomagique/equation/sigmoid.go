package equation

import (
	"math"

	"github.com/theapemachine/symm/nomagique/types"
)

/*
Sigmoid is the canonical logistic function σ(x) = 1/(1+e^{-x}).
Tier 4 Equation: pure transfer node, zero allocations, zero magic constants.
*/
type Sigmoid struct{}

func (Sigmoid) Step(x types.Scalar) types.Scalar {
	return types.Scalar(1.0 / (1.0 + math.Exp(-float64(x))))
}
