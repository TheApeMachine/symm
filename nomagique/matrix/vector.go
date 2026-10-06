package matrix

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Vector multiplies a matrix by a vector and retains the vector result. Each
arrival is *[2][][]float64 {matrix, {vector}}: the vector travels as the single
row of the second operand. It yields *[]float64.
*/
type Vector struct {
	*core.PrimitiveError
	out []float64
}

func NewVector() core.Primitive {
	return &Vector{
		PrimitiveError: core.NewPrimitiveError(),
	}
}

func (op *Vector) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			input := (*[2][][]float64)(arriving)

			if len(input[1]) != 1 {
				op.Error(core.ErrShape)
				return
			}

			matrix, vector := input[0], input[1][0]
			op.out = make([]float64, len(matrix))
			ok := true

			for rowIdx, row := range matrix {
				if len(row) != len(vector) {
					op.Error(core.ErrShape)
					ok = false
					break
				}

				var sum float64

				for colIdx, val := range row {
					sum += val * vector[colIdx]
				}

				op.out[rowIdx] = sum
			}

			if !ok {
				return
			}

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}
