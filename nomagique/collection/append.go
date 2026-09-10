package collection

import (
	"iter"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Append owns extending a collection. Configuration supplies the value a run
starts from. What it hands over is the collection after every arrival.
*/
type Append[T any] struct {
	core.Base[T, []T]
}

func NewAppend[T any](current []T) *Append[T] {
	op := &Append[T]{}
	op.Carrier(current)
	return op
}

func (op *Append[T]) Next(
	in iter.Seq[core.Primitive[T, T]],
) iter.Seq[core.Primitive[[]T, []T]] {
	return func(yield func(core.Primitive[[]T, []T]) bool) {
		for arriving := range in {
			if !yield(op.Carrier(append(op.Read(), arriving.Read()))) {
				return
			}
		}
	}
}
