package transport

import (
	"iter"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Indexed is a value and where it fell in its run. The index is structural
transport, not a domain count or a statistical support estimate.
*/
type Indexed[T any] struct {
	Index int
	Value T
}

/*
Enumerate attaches a run-relative index to each value. The index restarts with
every run because it describes a position within one delivery.
*/
type Enumerate[T any] struct {
	core.Base[T, Indexed[T]]
}

func NewEnumerate[T any]() *Enumerate[T] {
	return &Enumerate[T]{}
}

func (op *Enumerate[T]) Next(
	in iter.Seq[core.Primitive[T, T]],
) iter.Seq[core.Primitive[Indexed[T], Indexed[T]]] {
	return func(yield func(core.Primitive[Indexed[T], Indexed[T]]) bool) {
		index := 0

		for arriving := range in {
			if !yield(op.Carrier(Indexed[T]{Index: index, Value: arriving.Read()})) {
				return
			}

			index++
		}
	}
}
