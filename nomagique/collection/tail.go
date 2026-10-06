package collection

import (
	"iter"
	"slices"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Tail selects the last configured number of collection members.
*/
type Tail[T any] struct {
	*core.PrimitiveError
	capacity int
	out      []T
}

func NewTail[T any](capacity int) core.Primitive {
	op := &Tail[T]{
		PrimitiveError: core.NewPrimitiveError(),
		capacity:       capacity,
	}

	if capacity < 0 {
		op.Error(core.ErrShape)
	}

	return op
}

func (op *Tail[T]) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		if op.Error() != nil {
			return
		}

		for arriving := range in {
			values := *(*[]T)(arriving)
			start := max(0, len(values)-op.capacity)
			op.out = slices.Clone(values[start:])

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}
