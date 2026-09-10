package transport

import (
	"iter"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Fan presents one input run to every configured branch and streams what each
branch yields. The sequence is ranged once per branch; a replayable producer
(Values, a fold over a collection) supplies the same run to each, and a
one-shot producer is ranged as many times as it will produce.

Buffering a one-shot run so every branch sees a snapshot is Collect, not Fan.
*/
type Fan[T any] struct {
	core.Base[T, T]
	branches []core.Primitive[T, T]
}

func NewFan[T any](branches ...core.Primitive[T, T]) *Fan[T] {
	return &Fan[T]{branches: branches}
}

func (op *Fan[T]) Next(
	in iter.Seq[core.Primitive[T, T]],
) iter.Seq[core.Primitive[T, T]] {
	return func(yield func(core.Primitive[T, T]) bool) {
		for _, branch := range op.branches {
			for out := range branch.Next(in) {
				if !yield(out) {
					return
				}
			}
		}
	}
}
