package arithmetic

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
AugmentInput is two matrices joined horizontally.
*/
type AugmentInput struct {
	Left  [][]float64
	Right [][]float64
}

/*
Augment joins corresponding rows. Unequal row counts are a shape error.
*/
type Augment struct {
	*core.PrimitiveError

	out [][]float64
}

func NewAugment() *Augment {
	return &Augment{PrimitiveError: core.NewPrimitiveError()}
}

func (augment *Augment) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			input := (*AugmentInput)(arriving)

			if len(input.Left) != len(input.Right) {
				augment.Error(core.ErrShape)
				return
			}

			augment.out = make([][]float64, len(input.Left))

			for index, left := range input.Left {
				joined := make([]float64, 0, len(left)+len(input.Right[index]))
				joined = append(joined, left...)
				joined = append(joined, input.Right[index]...)
				augment.out[index] = joined
			}

			if !yield(unsafe.Pointer(&augment.out)) {
				return
			}
		}
	}
}
