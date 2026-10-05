package linear

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

type Matrix struct {
	*core.PrimitiveError
	rows int
	cols int
}

func NewMatrix(rows, cols int) core.Primitive {
	return &Matrix{
		PrimitiveError: core.NewPrimitiveError(),
		rows:           rows,
		cols:           cols,
	}
}

func (op *Matrix) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		out := make([]float64, op.rows)

		for arriving := range in {
			// A pair of slices arrives: pair[0] is matrix (rows*cols), pair[1] is vector (cols)
			pair := (*[2][]float64)(arriving)
			mat := pair[0]
			vec := pair[1]

			if len(mat) != op.rows*op.cols || len(vec) != op.cols {
				op.Error(core.ErrShape)
				return
			}

			for r := 0; r < op.rows; r++ {
				sum := 0.0
				offset := r * op.cols
				for c := 0; c < op.cols; c++ {
					sum += mat[offset+c] * vec[c]
				}
				out[r] = sum
			}

			if !yield(unsafe.Pointer(&out)) {
				return
			}
		}
	}
}
