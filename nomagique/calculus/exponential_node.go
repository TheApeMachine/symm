package calculus

import (
	"math"

	"github.com/theapemachine/symm/nomagique/types"
)

/*
ExponentialNode is a pure non-linear transfer function e^(-x).
*/
type ExponentialNode struct{}

func (exponential ExponentialNode) Step(number types.Number) types.Number {
	return types.Number(math.Exp(-float64(number)))
}
