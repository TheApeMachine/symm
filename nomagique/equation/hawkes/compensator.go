package hawkes

import (
	"iter"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
CompensatorInput is the window integral of both intensities.
*/
type CompensatorInput struct {
	Parameters
	Span      float64
	IntegralX float64
	IntegralY float64
}

/*
Compensator integrates both intensities across the observed window.
*/
type Compensator struct {
	core.Base[CompensatorInput, float64]
}

func NewCompensator() *Compensator {
	return &Compensator{}
}

func (op *Compensator) Next(
	in iter.Seq[core.Primitive[CompensatorInput, CompensatorInput]],
) iter.Seq[core.Primitive[float64, float64]] {
	return func(yield func(core.Primitive[float64, float64]) bool) {
		for arriving := range in {
			input := arriving.Read()
			value := (input.MuX+input.MuY)*input.Span +
				((input.AlphaXX+input.AlphaYX)/input.Beta)*input.IntegralX +
				((input.AlphaXY+input.AlphaYY)/input.Beta)*input.IntegralY

			if !yield(op.Carrier(value)) {
				return
			}
		}
	}
}
