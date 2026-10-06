package matrix

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Augment joins corresponding rows. Each arrival is *[2][][]float64
{left, right}; it yields *[][]float64. Unequal row counts are a shape error.
*/
type Augment struct {
	*core.PrimitiveError
	out [][]float64
}

func NewAugment() core.Primitive {
	return &Augment{
		PrimitiveError: core.NewPrimitiveError(),
	}
}

func (op *Augment) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			input := (*[2][][]float64)(arriving)
			left, right := input[0], input[1]

			if len(left) != len(right) {
				op.Error(core.ErrShape)
				return
			}

			op.out = make([][]float64, len(left))

			for index, left := range left {
				joined := make([]float64, 0, len(left)+len(right[index]))
				joined = append(joined, left...)
				joined = append(joined, right[index]...)
				op.out[index] = joined
			}

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}
