package transport

import (
	"iter"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Pass hands each arrival over unchanged. It is the identity stage: Fan, Pipe,
and tests compose against a Primitive, not against a missing stage.
*/
type Pass[T any] struct {
	core.Base[T, T]
}

func NewPass[T any]() *Pass[T] {
	return &Pass[T]{}
}

func (op *Pass[T]) Next(
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
