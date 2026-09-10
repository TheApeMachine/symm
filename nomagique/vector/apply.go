package vector

import (
	"iter"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
Apply aligns one arrival with each configured operation and forwards that
operation's complete output. Operations retain their identity between runs, so
each coordinate may own an independent recurrence.
*/
type Apply[T, U any] struct {
	core.Base[T, U]
	operations []core.Primitive[T, U]
}

func NewApply[T, U any](operations ...core.Primitive[T, U]) *Apply[T, U] {
	return &Apply[T, U]{operations: operations}
}

func (op *Apply[T, U]) Next(
	in iter.Seq[core.Primitive[T, T]],
) iter.Seq[core.Primitive[U, U]] {
	return func(yield func(core.Primitive[U, U]) bool) {
		index := 0

		for arriving := range in {
			if index >= len(op.operations) {
				op.Error(core.ErrShape)
				return
			}

			for out := range op.operations[index].Next(transport.One(arriving)) {
				if !yield(op.Carrier(out.Read())) {
					return
				}
			}

			op.Error(op.operations[index].Error())
			index++
		}

		if index != len(op.operations) {
			op.Error(core.ErrShape)
		}
	}
}
