package logic

import (
	"iter"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Pick owns selection of one candidate. The predicate sees the held value and the
arrival as a pair and decides whether the arrival replaces what is held.
*/
type Pick[T any] struct {
	core.Base[T, T]
	predicate core.Primitive[[2]T, bool]
	held      bool
}

func NewPick[T any](predicate core.Primitive[[2]T, bool]) *Pick[T] {
	return &Pick[T]{predicate: predicate}
}

func (op *Pick[T]) Next(
	in iter.Seq[core.Primitive[T, T]],
) iter.Seq[core.Primitive[T, T]] {
	return func(yield func(core.Primitive[T, T]) bool) {
		for arriving := range in {
			value := arriving.Read()

			if !op.held {
				op.held = true

				if !yield(op.Carrier(value)) {
					return
				}

				continue
			}

			take := false
			pair := [2]T{op.Read(), value}

			for decision := range op.predicate.Next(func(yield func(core.Primitive[[2]T, [2]T]) bool) {
				carrier := &core.Carrier[[2]T]{}
				yield(carrier.Carrier(pair))
			}) {
				take = decision.Read()
			}

			chosen := op.Read()

			if take {
				chosen = value
			}

			if !yield(op.Carrier(chosen)) {
				return
			}
		}
	}
}
