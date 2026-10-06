package linear

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Matrix multiplies a row-major rows x cols matrix by a cols-long vector. Each
arrival is a *[2][]float64 {matrix, vector}; it yields the rows-long product
as a *[]float64.
*/
type Matrix struct {
	*core.PrimitiveError
	rows int
	cols int
}

func NewMatrix(rows, cols int) *Matrix {
	op := &Matrix{
		PrimitiveError: core.NewPrimitiveError(),
		rows:           rows,
		cols:           cols,
	}

	if rows < 0 || cols < 0 {
		op.Error(core.ErrShape)
	}

	return op
}

func (op *Matrix) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		if op.Error() != nil {
			return
		}

		out := make([]float64, op.rows)

		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

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
