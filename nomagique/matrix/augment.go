package matrix

import (
	"errors"
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
	err error
	out [][]float64
}

func NewAugment() core.Primitive {
	return &Augment{}
}

func (op *Augment) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			input := (*AugmentInput)(arriving)

			if len(input.Left) != len(input.Right) {
				op.Error(core.ErrShape)
				return
			}

			op.out = make([][]float64, len(input.Left))

			for index, left := range input.Left {
				joined := make([]float64, 0, len(left)+len(input.Right[index]))
				joined = append(joined, left...)
				joined = append(joined, input.Right[index]...)
				op.out[index] = joined
			}

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

func (op *Augment) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}
