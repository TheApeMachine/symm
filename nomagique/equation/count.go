package equation

import (
	"iter"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Count owns addition over unit contributions. It counts delivered objects,
regardless of their payload.
*/
type Count[T any] struct {
	core.Base[T, float64]
}

func NewCount[T any]() *Count[T] {
	op := &Count[T]{}
	op.Carrier(0)
	return op
}

func (op *Count[T]) Next(
	in iter.Seq[core.Primitive[T, T]],
) iter.Seq[core.Primitive[float64, float64]] {
	return func(yield func(core.Primitive[float64, float64]) bool) {
		for range in {
			if !yield(op.Carrier(op.Read() + 1)) {
				return
			}
		}
	}
}
