package collection

import (
	"fmt"
	"iter"
	"slices"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Swap exchanges two indexed members without mutating its input collection.
Both indices are configuration.
*/
type Swap[T any] struct {
	core.Base[[]T, []T]
	left  int
	right int
}

func NewSwap[T any](left, right int) *Swap[T] {
	return &Swap[T]{left: left, right: right}
}

func (op *Swap[T]) Next(
	in iter.Seq[core.Primitive[[]T, []T]],
) iter.Seq[core.Primitive[[]T, []T]] {
	return func(yield func(core.Primitive[[]T, []T]) bool) {
		for arriving := range in {
			values := arriving.Read()

			if op.left < 0 || op.left >= len(values) || op.right < 0 || op.right >= len(values) {
				op.Error(fmt.Errorf("%w: swap %d, %d of %d", core.ErrShape, op.left, op.right, len(values)))
				continue
			}

			updated := slices.Clone(values)
			updated[op.left], updated[op.right] = updated[op.right], updated[op.left]

			if !yield(op.Carrier(updated)) {
				return
			}
		}
	}
}
