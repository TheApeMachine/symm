package hawkes

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/store"
)

// NewIntegralDerivative is the derivative of the integral-support numerator
// with respect to beta, retaining both boundary-age terms.
func NewIntegralDerivative(side core.Primitive) core.Primitive {
	return NewIntegral(
		side,
		equation.NewDifference[float64](
			equation.NewProduct[float64](store.NewGet("upper"), NewKernel(store.NewGet("beta"), store.NewGet("upper"))),
			equation.NewProduct[float64](store.NewGet("lower"), NewKernel(store.NewGet("beta"), store.NewGet("lower"))),
		),
	)
}
