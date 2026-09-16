package arithmetic

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
	*core.PrimitiveError

	indices []int
	out     []float64
}

func NewDesign(indices ...int) *Design {
	return &Design{PrimitiveError: core.NewPrimitiveError(), indices: indices}
}

func (design *Design) Next(
	in iter.Seq[unsafe.Pointer],
) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			features := *(*[]float64)(arriving)

			if len(design.indices) == 0 {
				design.out = make([]float64, 0, len(features)+1)
				design.out = append(design.out, 1.0)
				design.out = append(design.out, features...)
			} else {
				design.out = make([]float64, 0, len(design.indices)+1)
				design.out = append(design.out, 1.0)

				for _, idx := range design.indices {
					if idx < 0 || idx >= len(features) {
						design.Error(core.ErrShape)
						return
					}

					design.out = append(design.out, features[idx])
				}
			}

			if !yield(unsafe.Pointer(&design.out)) {
				return
			}
		}
	}
}
