package collection

import (
	"iter"
	"slices"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Tail selects the last configured number of collection members. Capacity is
configuration, never inferred from the payload.
*/
type Tail[T any] struct {
	core.Base[[]T, []T]
	capacity int
}

func NewTail[T any](capacity int) *Tail[T] {
	op := &Tail[T]{capacity: capacity}

	if capacity < 0 {
		op.Error(core.ErrShape)
	}

	return op
}

func (op *Tail[T]) Next(
	in iter.Seq[core.Primitive[[]T, []T]],
) iter.Seq[core.Primitive[[]T, []T]] {
	return func(yield func(core.Primitive[[]T, []T]) bool) {
		if op.Error() != nil {
			return
		}

		for arriving := range in {
			values := arriving.Read()
			start := max(0, len(values)-op.capacity)

			if !yield(op.Carrier(slices.Clone(values[start:]))) {
				return
			}
		}
	}
}
