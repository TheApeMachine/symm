package collection

import (
	"cmp"
	"iter"
	"slices"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Order owns ordering a collection. It never reorders a caller's storage.
*/
type Order[T cmp.Ordered] struct {
	core.Base[[]T, []T]
}

func NewOrder[T cmp.Ordered]() *Order[T] {
	return &Order[T]{}
}

func (op *Order[T]) Next(
	in iter.Seq[core.Primitive[[]T, []T]],
) iter.Seq[core.Primitive[[]T, []T]] {
	return func(yield func(core.Primitive[[]T, []T]) bool) {
		for arriving := range in {
			ordered := slices.Clone(arriving.Read())
			slices.Sort(ordered)

			if !yield(op.Carrier(ordered)) {
				return
			}
		}
	}
}
