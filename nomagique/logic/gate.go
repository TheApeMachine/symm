package logic

import (
	"iter"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Gate routes each arrival through one of two operations according to a
predicate. The predicate and the branches are themselves Primitives; Gate does
not snapshot a run in order to replay it.
*/
type Gate[T, U any] struct {
	core.Base[T, U]
	predicate core.Primitive[T, bool]
	pass      core.Primitive[T, U]
	fail      core.Primitive[T, U]
}

func NewGate[T, U any](
	predicate core.Primitive[T, bool],
	pass, fail core.Primitive[T, U],
) *Gate[T, U] {
	return &Gate[T, U]{predicate: predicate, pass: pass, fail: fail}
}

func (op *Gate[T, U]) Next(
	in iter.Seq[core.Primitive[T, T]],
) iter.Seq[core.Primitive[U, U]] {
	return func(yield func(core.Primitive[U, U]) bool) {
		for arriving := range in {
			once := func(yield func(core.Primitive[T, T]) bool) {
				yield(arriving)
			}

			selected := false

			for decision := range op.predicate.Next(once) {
				selected = decision.Read()
			}

			branch := op.fail

			if selected {
				branch = op.pass
			}

			for out := range branch.Next(once) {
				if !yield(op.Carrier(out.Read())) {
					return
				}
			}
		}
	}
}
