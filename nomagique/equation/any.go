package equation

import (
	"iter"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
Any is the disjunction of configured predicates on each arrival. The empty
disjunction is false.
*/
type Any[T any] struct {
	core.Base[T, bool]
	predicates []core.Primitive[T, bool]
}

func NewAny[T any](predicates ...core.Primitive[T, bool]) *Any[T] {
	return &Any[T]{predicates: predicates}
}

func (op *Any[T]) Next(
	in iter.Seq[core.Primitive[T, T]],
) iter.Seq[core.Primitive[bool, bool]] {
	return func(yield func(core.Primitive[bool, bool]) bool) {
		for arriving := range in {
			ok := false

			for _, predicate := range op.predicates {
				for decision := range predicate.Next(transport.One(arriving)) {
					ok = ok || decision.Read()
				}
			}

			if !yield(op.Carrier(ok)) {
				return
			}
		}
	}
}
