package store

import (
	"iter"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Retained holds the latest arrival. Configuration supplies the value before the
first update. Read is the query; Next is the update.
*/
type Retained[T any] struct {
	core.Base[T, T]
}

func NewRetained[T any](current T) *Retained[T] {
	op := &Retained[T]{}
	op.Carrier(current)
	return op
}

func (op *Retained[T]) Next(
	in iter.Seq[core.Primitive[T, T]],
) iter.Seq[core.Primitive[T, T]] {
	return func(yield func(core.Primitive[T, T]) bool) {
		for arriving := range in {
			if !yield(op.Carrier(arriving.Read())) {
				return
			}
		}
	}
}
