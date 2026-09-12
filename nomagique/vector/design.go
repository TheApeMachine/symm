package vector

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Design selects configured feature positions and prepends the affine intercept.
Its output is a design vector; it does not fit or predict.
*/
type Design struct {
	err     error
	indices []int
	out     []float64
}

func NewDesign(indices ...int) core.Primitive {
	return &Design{indices: indices}
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
			} else {
				op.out = make([]float64, 0, len(op.indices)+1)
				op.out = append(op.out, 1.0)

				for _, idx := range op.indices {
					if idx < 0 || idx >= len(features) {
						op.err = core.ErrShape
						return
					}

					op.out = append(op.out, features[idx])
				}
			}

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

func (op *Design) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = err
			break
		}
	}

	return op.err
}
