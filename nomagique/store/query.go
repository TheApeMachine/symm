package store

import (
	"iter"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Query holds the latest arrival of a question. Selector-then-data ordering is
the caller's composition, not a second protocol inside Query.
*/
type Query[T any] struct {
	core.Base[T, T]
}

func NewQuery[T any](current T) *Query[T] {
	op := &Query[T]{}
	op.Carrier(current)
	return op
}

func (op *Query[T]) Next(
	in iter.Seq[core.Primitive[T, T]],
) iter.Seq[core.Primitive[T, T]] {
	return func(yield func(core.Primitive[T, T]) bool) {
		for arriving := range in {
			if !yield(op.Carrier(arriving.Read())) {
				return
			}
		}
	}
}
