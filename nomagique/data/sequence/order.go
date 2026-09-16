package sequence

import (
	"cmp"
	"iter"
	"slices"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Order owns ordering a collection.
*/
type Order[T cmp.Ordered] struct {
	*core.PrimitiveError

	out []T
}

func NewOrder[T cmp.Ordered]() *Order[T] {
	return &Order[T]{PrimitiveError: core.NewPrimitiveError()}
}

func (order *Order[T]) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			ordered := slices.Clone(*(*[]T)(arriving))
			slices.Sort(ordered)
			order.out = ordered

			if !yield(unsafe.Pointer(&order.out)) {
				return
			}
		}
	}
}
