package equation

import (
	"iter"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
All is the conjunction of configured predicates on each arrival. The empty
conjunction is true.
*/
type All[T any] struct {
	core.Base[T, bool]
	predicates []core.Primitive[T, bool]
}

func NewAll[T any](predicates ...core.Primitive[T, bool]) *All[T] {
	return &All[T]{predicates: predicates}
}

func (op *All[T]) Next(
	in iter.Seq[core.Primitive[T, T]],
) iter.Seq[core.Primitive[bool, bool]] {
	return func(yield func(core.Primitive[bool, bool]) bool) {
		for arriving := range in {
			ok := true

			for _, predicate := range op.predicates {
				for decision := range predicate.Next(transport.One(arriving)) {
					ok = ok && decision.Read()
				}
			}

			if !yield(op.Carrier(ok)) {
				return
			}
		}
	}
}
