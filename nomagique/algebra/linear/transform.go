package linear

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

type Transform struct {
	*core.PrimitiveError
	rows    int
	cols    int
	weights []float64
}

func NewTransform(rows, cols int, weights []float64) core.Primitive {
	return &Transform{
		PrimitiveError: core.NewPrimitiveError(),
		rows:           rows,
		cols:           cols,
		weights:        weights,
	}
}

func (op *Transform) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		out := make([]float64, op.rows)

		for arriving := range in {
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
