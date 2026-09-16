package arithmetic

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
MatrixScaleInput is a matrix and the scalar that multiplies every coefficient.
*/
type MatrixScaleInput struct {
	Values [][]float64
	Factor float64
}

/*
MatrixScale multiplies typed matrix coefficients by one scalar.
*/
type MatrixScale struct {
	*core.PrimitiveError

	out [][]float64
}

func NewMatrixScale() *MatrixScale {
	return &MatrixScale{PrimitiveError: core.NewPrimitiveError()}
}

func (matrixScale *MatrixScale) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			input := (*MatrixScaleInput)(arriving)
			matrixScale.out = make([][]float64, len(input.Values))

			for row, values := range input.Values {
				matrixScale.out[row] = make([]float64, len(values))

				for column, value := range values {
					matrixScale.out[row][column] = value * input.Factor
				}
			}

			if !yield(unsafe.Pointer(&matrixScale.out)) {
				return
			}
		}
	}
}
