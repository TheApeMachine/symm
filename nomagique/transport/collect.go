package transport

import (
	"iter"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Collect retains the values of one run as one collection.

It is the one shape that cannot stream: a collection is not complete until its
run is spent, so nothing is handed over until everything has arrived. That is a
property of the operation, not of the contract.
*/
type Collect[T any] struct {
	core.Base[T, []T]
}

func NewCollect[T any]() *Collect[T] {
	return &Collect[T]{}
}

func (op *Collect[T]) Next(
	in iter.Seq[core.Primitive[T, T]],
) iter.Seq[core.Primitive[[]T, []T]] {
	return func(yield func(core.Primitive[[]T, []T]) bool) {
		var gathered []T

		for arriving := range in {
			gathered = append(gathered, arriving.Read())
		}

		if !yield(op.Carrier(gathered)) {
			return
		}
	}
}
