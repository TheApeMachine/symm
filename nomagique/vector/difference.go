package vector

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Difference subtracts paired members. Each arrival is *[2][]float64
{left, right}; it yields *[]float64. Unequal lengths are a shape error.
*/
type Difference struct {
	*core.PrimitiveError
	out []float64
}

func NewDifference() core.Primitive {
	return &Difference{
		PrimitiveError: core.NewPrimitiveError(),
	}
}

func (op *Difference) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			pair := (*[2][]float64)(arriving)
			left, right := pair[0], pair[1]

			if len(left) != len(right) {
				op.Error(core.ErrShape)
				continue
			}

			if len(op.out) != len(left) {
				op.out = make([]float64, len(left))
			}

			for index, value := range left {
				op.out[index] = value - right[index]
			}

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}
