package transport

import (
	"iter"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Spread presents collection members as individual yields. Collection shape
conversion belongs here. A member is handed over as soon as it is read from an
incoming collection.

Spread is Primitive[[]T, T]: what arrives is a collection, what it hands
downstream is one member.
*/
type Spread[T any] struct {
	core.Base[[]T, T]
}

func NewSpread[T any]() *Spread[T] {
	return &Spread[T]{}
}

func (op *Spread[T]) Next(
	in iter.Seq[core.Primitive[[]T, []T]],
) iter.Seq[core.Primitive[T, T]] {
	return func(yield func(core.Primitive[T, T]) bool) {
		for collection := range in {
			for _, member := range collection.Read() {
				if !yield(op.Carrier(member)) {
					return
				}
			}
		}
	}
}
