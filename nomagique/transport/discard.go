package transport

import (
	"iter"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Discard consumes a run without handing anything over.

A run that nobody ranges never happens, so discarding is an operation: it is
what asks a stage to do its work when the work is the point and the values are
not.
*/
type Discard[T any] struct {
	core.Base[T, T]
}

func NewDiscard[T any]() *Discard[T] {
	return &Discard[T]{}
}

func (op *Discard[T]) Next(
	in iter.Seq[core.Primitive[T, T]],
) iter.Seq[core.Primitive[T, T]] {
	return func(func(core.Primitive[T, T]) bool) {
		for range in {
		}
	}
}
