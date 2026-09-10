package store

import (
	"iter"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
First is the leading member of a pair. Pairs travel as one value, so taking
one member is the smallest operation that takes them apart.
*/
type First[T any] struct {
	core.Base[[2]T, T]
}

func NewFirst[T any]() *First[T] {
	return &First[T]{}
}

func (op *First[T]) Next(
	in iter.Seq[core.Primitive[[2]T, [2]T]],
) iter.Seq[core.Primitive[T, T]] {
	return func(yield func(core.Primitive[T, T]) bool) {
		for arriving := range in {
			if !yield(op.Carrier(arriving.Read()[0])) {
				return
			}
		}
	}
}
