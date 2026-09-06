package hawkes

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/store"
)

// NewIntegralSupport composes the exponential compensator numerator per side.
// Divide by beta in the compensator, not here: this matches the source units.
func NewIntegralSupport(side core.Primitive) core.Primitive {
	return NewIntegral(
		side,
		equation.NewDifference[float64](
			NewKernel(store.NewGet("beta"), store.NewGet("lower")),
			NewKernel(store.NewGet("beta"), store.NewGet("upper")),
		),
	)
}
