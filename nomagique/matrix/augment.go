package matrix

import (
	"iter"

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
	core.Base[AugmentInput, [][]float64]
}

func NewAugment() *Augment {
	return &Augment{}
}

func (op *Augment) Next(
	in iter.Seq[core.Primitive[AugmentInput, AugmentInput]],
) iter.Seq[core.Primitive[[][]float64, [][]float64]] {
	return func(yield func(core.Primitive[[][]float64, [][]float64]) bool) {
		for arriving := range in {
			input := arriving.Read()

			if len(input.Left) != len(input.Right) {
				op.Error(core.ErrShape)
				continue
			}

			rows := make([][]float64, len(input.Left))

			for index, left := range input.Left {
				joined := make([]float64, 0, len(left)+len(input.Right[index]))
				joined = append(joined, left...)
				joined = append(joined, input.Right[index]...)
				rows[index] = joined
			}

			if !yield(op.Carrier(rows)) {
				return
			}
		}
	}
}
