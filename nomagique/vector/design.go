package vector

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Design selects configured feature positions and prepends the affine intercept.
Each arrival is *[]float64 features; it yields *[]float64, a design vector.
It does not fit or predict.
*/
type Design struct {
	*core.PrimitiveError
	indices []int
	out     []float64
}

func NewDesign(indices ...int) core.Primitive {
	return &Design{
		PrimitiveError: core.NewPrimitiveError(),
		indices:        indices,
	}
}

func (op *Design) Next(
	in iter.Seq[unsafe.Pointer],
) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			features := *(*[]float64)(arriving)

			if len(op.indices) == 0 {
				op.out = make([]float64, 0, len(features)+1)
				op.out = append(op.out, 1.0)
				op.out = append(op.out, features...)

				if !yield(unsafe.Pointer(&op.out)) {
					return
				}

				continue
			}

			op.out = make([]float64, 0, len(op.indices)+1)
			op.out = append(op.out, 1.0)

			for _, idx := range op.indices {
				if idx < 0 || idx >= len(features) {
					op.Error(core.ErrShape)
					return
				}

				op.out = append(op.out, features[idx])
			}

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}
