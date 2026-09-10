package matrix

import (
	"iter"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
ScaleInput is a matrix and the scalar that multiplies every coefficient.
*/
type ScaleInput struct {
	Values [][]float64
	Factor float64
}

/*
Scale multiplies typed matrix coefficients by one scalar.
*/
type Scale struct {
	core.Base[ScaleInput, [][]float64]
}

func NewScale() *Scale {
	return &Scale{}
}

func (op *Scale) Next(
	in iter.Seq[core.Primitive[ScaleInput, ScaleInput]],
) iter.Seq[core.Primitive[[][]float64, [][]float64]] {
	return func(yield func(core.Primitive[[][]float64, [][]float64]) bool) {
		for arriving := range in {
			input := arriving.Read()
			scaled := make([][]float64, len(input.Values))

			for row, values := range input.Values {
				scaled[row] = make([]float64, len(values))

				for column, value := range values {
					scaled[row][column] = value * input.Factor
				}
			}

			if !yield(op.Carrier(scaled)) {
				return
			}
		}
	}
}
