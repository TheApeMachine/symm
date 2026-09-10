package collection

import (
	"fmt"
	"iter"
	"slices"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Set replaces one indexed member without mutating its input collection. Index
and replacement are configuration.
*/
type Set[T any] struct {
	core.Base[[]T, []T]
	index int
	value T
}

func NewSet[T any](index int, value T) *Set[T] {
	return &Set[T]{index: index, value: value}
}

func (op *Set[T]) Next(
	in iter.Seq[core.Primitive[[]T, []T]],
) iter.Seq[core.Primitive[[]T, []T]] {
	return func(yield func(core.Primitive[[]T, []T]) bool) {
		for arriving := range in {
			values := arriving.Read()

			if op.index < 0 || op.index >= len(values) {
				op.Error(fmt.Errorf("%w: index %d of %d", core.ErrShape, op.index, len(values)))
				continue
			}

			updated := slices.Clone(values)
			updated[op.index] = op.value

			if !yield(op.Carrier(updated)) {
				return
			}
		}
	}
}
