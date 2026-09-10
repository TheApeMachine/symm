package collection

import (
	"fmt"
	"iter"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
At selects an indexed member. The index is configuration, not a second
Primitive graph.
*/
type At[T any] struct {
	core.Base[[]T, T]
	index int
}

func NewAt[T any](index int) *At[T] {
	return &At[T]{index: index}
}

func (op *At[T]) Next(
	in iter.Seq[core.Primitive[[]T, []T]],
) iter.Seq[core.Primitive[T, T]] {
	return func(yield func(core.Primitive[T, T]) bool) {
		for arriving := range in {
			values := arriving.Read()

			if op.index < 0 || op.index >= len(values) {
				op.Error(fmt.Errorf("%w: index %d of %d", core.ErrShape, op.index, len(values)))
				continue
			}

			if !yield(op.Carrier(values[op.index])) {
				return
			}
		}
	}
}
