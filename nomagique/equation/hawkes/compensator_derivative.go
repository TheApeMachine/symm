package hawkes

import (
	"iter"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
CompensatorDerivativeInput includes both kernel-numerator and 1/beta scale
derivatives.
*/
type CompensatorDerivativeInput struct {
	Parameters
	IntegralX     float64
	IntegralY     float64
	IntegralXBeta float64
	IntegralYBeta float64
}

/*
CompensatorDerivative includes the derivative of both the kernel numerator
and the 1/beta scale.
*/
type CompensatorDerivative struct {
	core.Base[CompensatorDerivativeInput, float64]
}

func NewCompensatorDerivative() *CompensatorDerivative {
	return &CompensatorDerivative{}
}

func (op *CompensatorDerivative) Next(
	in iter.Seq[core.Primitive[CompensatorDerivativeInput, CompensatorDerivativeInput]],
) iter.Seq[core.Primitive[float64, float64]] {
	return func(yield func(core.Primitive[float64, float64]) bool) {
		for arriving := range in {
			input := arriving.Read()
			value := (input.AlphaXX+input.AlphaYX)*(input.IntegralXBeta/input.Beta-input.IntegralX/(input.Beta*input.Beta)) +
				(input.AlphaXY+input.AlphaYY)*(input.IntegralYBeta/input.Beta-input.IntegralY/(input.Beta*input.Beta))

			if !yield(op.Carrier(value)) {
				return
			}
		}
	}
}
