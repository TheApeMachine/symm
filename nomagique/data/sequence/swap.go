package sequence

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

func NewSwap[T any](left, right int) *Swap[T] {
	return &Swap[T]{PrimitiveError: core.NewPrimitiveError(), left: left, right: right}
}

func (swap *Swap[T]) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			values := *(*[]T)(arriving)

			if swap.left < 0 || swap.left >= len(values) || swap.right < 0 || swap.right >= len(values) {
				swap.Error(fmt.Errorf("%w: swap %d, %d of %d", core.ErrShape, swap.left, swap.right, len(values)))
				return
			}

			updated := slices.Clone(values)
			updated[swap.left], updated[swap.right] = updated[swap.right], updated[swap.left]
			swap.out = updated

			if !yield(unsafe.Pointer(&swap.out)) {
				return
			}
		}
	}
}
