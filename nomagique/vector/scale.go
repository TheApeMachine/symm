package vector

import (
	"iter"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
ScaleInput is a vector and the scalar that multiplies every member.
*/
type ScaleInput struct {
	Values []float64
	Factor float64
}

/*
Scale multiplies each member by one scalar.
*/
type Scale struct {
	core.Base[ScaleInput, []float64]
}

func NewScale() *Scale {
	return &Scale{}
}

func (op *Scale) Next(
	in iter.Seq[core.Primitive[ScaleInput, ScaleInput]],
) iter.Seq[core.Primitive[[]float64, []float64]] {
	return func(yield func(core.Primitive[[]float64, []float64]) bool) {
		for arriving := range in {
			input := arriving.Read()
			out := make([]float64, len(input.Values))

			for index, value := range input.Values {
				out[index] = value * input.Factor
			}

			if !yield(op.Carrier(out)) {
				return
			}
		}
	}
}
