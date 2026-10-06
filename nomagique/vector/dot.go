package vector

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Dot owns the inner product of two equal-length vectors. Each arrival is
*[2][]float64 {left, right}; it yields *float64.
*/
type Dot struct {
	*core.PrimitiveError
	out float64
}

func NewDot() core.Primitive {
	return &Dot{
		PrimitiveError: core.NewPrimitiveError(),
	}
}

func (op *Dot) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			pair := (*[2][]float64)(arriving)
			left, right := pair[0], pair[1]

			if len(left) != len(right) {
				op.Error(core.ErrShape)
				continue
			}

			op.out = 0.0
			for index, value := range left {
				op.out += value * right[index]
			}

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}
