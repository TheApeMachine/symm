package store

import (
	"iter"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Second is the trailing member of a pair, and the counterpart of First.
*/
type Second[T any] struct {
	core.Base[[2]T, T]
}

func NewSecond[T any]() *Second[T] {
	return &Second[T]{}
}

func (op *Second[T]) Next(
	in iter.Seq[core.Primitive[[2]T, [2]T]],
) iter.Seq[core.Primitive[T, T]] {
	return func(yield func(core.Primitive[T, T]) bool) {
		for arriving := range in {
			if !yield(op.Carrier(arriving.Read()[1])) {
				return
			}
		}
	}
}
