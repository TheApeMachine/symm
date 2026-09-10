package transport

import (
	"iter"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Apply binds a run to a target.

The target is asked with the bound run rather than with whatever arrives. What
arrives is not consumed and not forwarded: the binding replaces it.
*/
type Apply[T, U any] struct {
	core.Base[T, U]
	target core.Primitive[T, U]
	bound  iter.Seq[core.Primitive[T, T]]
}

func NewApply[T, U any](
	target core.Primitive[T, U],
	bound iter.Seq[core.Primitive[T, T]],
) *Apply[T, U] {
	return &Apply[T, U]{target: target, bound: bound}
}

func (op *Apply[T, U]) Next(
	iter.Seq[core.Primitive[T, T]],
) iter.Seq[core.Primitive[U, U]] {
	return func(yield func(core.Primitive[U, U]) bool) {
		for out := range op.target.Next(op.bound) {
			if !yield(op.Carrier(out.Read())) {
				return
			}
		}
	}
}
