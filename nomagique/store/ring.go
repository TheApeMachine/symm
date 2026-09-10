package store

import (
	container "container/ring"
	"iter"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Ring plays out a ring of rings. Each Next run is one child sequence. When that
child is spent the run ends, and the next Next begins at the parent's next
child. The parent loops, so the sequences replay and their order never restarts.
*/
type Ring[T any] struct {
	core.Base[T, T]
	parent *container.Ring
	child  *container.Ring
	played int
}

func NewRing[T any](parent *container.Ring) *Ring[T] {
	return &Ring[T]{parent: parent}
}

func (op *Ring[T]) Next(
	iter.Seq[core.Primitive[T, T]],
) iter.Seq[core.Primitive[T, T]] {
	return func(yield func(core.Primitive[T, T]) bool) {
		if op.parent == nil {
			return
		}

		child, held := op.parent.Value.(*container.Ring)

		if !held || child == nil {
			return
		}

		op.child, op.played = child, 0

		for op.played < op.child.Len() {
			value, ok := op.child.Value.(T)
			op.child, op.played = op.child.Next(), op.played+1

			if !ok {
				return
			}

			if !yield(op.Carrier(value)) {
				return
			}
		}

		op.parent = op.parent.Next()
	}
}
