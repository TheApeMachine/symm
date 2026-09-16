package arithmetic

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
DifferenceInput is a pair of equally shaped matrices.
*/
type DifferenceInput struct {
	Left  [][]float64
	Right [][]float64
}

/*
MatrixDifference subtracts equally shaped matrices in typed coefficient storage.
*/
type MatrixDifference struct {
	*core.PrimitiveError

	out [][]float64
}

func NewMatrixDifference() *MatrixDifference {
	return &MatrixDifference{PrimitiveError: core.NewPrimitiveError()}
}

func (matrixDifference *MatrixDifference) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			input := (*DifferenceInput)(arriving)

			if len(input.Left) != len(input.Right) {
				matrixDifference.Error(core.ErrShape)
				return
			}

			matrixDifference.out = make([][]float64, len(input.Left))
			ok := true

			for row, values := range input.Left {
				if len(values) != len(input.Right[row]) {
					matrixDifference.Error(core.ErrShape)
					ok = false
					break
				}

				matrixDifference.out[row] = make([]float64, len(values))

				for column, value := range values {
					matrixDifference.out[row][column] = value - input.Right[row][column]
				}
			}

			if !ok {
				return
			}

			if !yield(unsafe.Pointer(&matrixDifference.out)) {
				return
			}
		}
	}
}
