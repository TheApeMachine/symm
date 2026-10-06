package collection

import (
	"fmt"
	"iter"
	"slices"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Swap exchanges two indexed members without mutating its input collection.
*/
type Swap[T any] struct {
	*core.PrimitiveError
	left  int
	right int
	out   []T
}

func NewSwap[T any](left, right int) core.Primitive {
	return &Swap[T]{
		PrimitiveError: core.NewPrimitiveError(),
		left:           left,
		right:          right,
	}
}

func (op *Swap[T]) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			values := *(*[]T)(arriving)

			if op.left < 0 || op.left >= len(values) || op.right < 0 || op.right >= len(values) {
				op.Error(fmt.Errorf("%w: swap %d, %d of %d", core.ErrShape, op.left, op.right, len(values)))
				return
			}

			updated := slices.Clone(values)
			updated[op.left], updated[op.right] = updated[op.right], updated[op.left]
			op.out = updated

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}
