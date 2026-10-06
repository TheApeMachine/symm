package linear

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Transform multiplies each arriving cols-long vector (*[]float64) by the
configured row-major rows x cols weights, and yields the rows-long product as
a *[]float64.
*/
type Transform struct {
	*core.PrimitiveError
	rows    int
	cols    int
	weights []float64
}

func NewTransform(rows, cols int, weights []float64) *Transform {
	op := &Transform{
		PrimitiveError: core.NewPrimitiveError(),
		rows:           rows,
		cols:           cols,
		weights:        append([]float64(nil), weights...),
	}

	if rows < 0 || cols < 0 || len(weights) != rows*cols {
		op.Error(core.ErrShape)
	}

	return op
}

func (op *Transform) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
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

			vec := *(*[]float64)(arriving)

			if len(vec) != op.cols {
				op.Error(core.ErrShape)
				return
			}

			for r := 0; r < op.rows; r++ {
				sum := 0.0
				offset := r * op.cols

				for c := 0; c < op.cols; c++ {
					sum += op.weights[offset+c] * vec[c]
				}

				out[r] = sum
			}

			if !yield(unsafe.Pointer(&out)) {
				return
			}
		}
	}
}
