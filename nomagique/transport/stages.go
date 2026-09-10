package transport

import (
	"iter"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Stages is a Primitive that threads a run through homogeneous stages. It exists
so a pipeline can be configured where a Primitive is required. Heterogeneous
composition remains nested Next calls.
*/
type Stages[T any] struct {
	core.Base[T, T]
	stages []core.Primitive[T, T]
}

func NewStages[T any](stages ...core.Primitive[T, T]) *Stages[T] {
	return &Stages[T]{stages: stages}
}

func (op *Stages[T]) Next(
	in iter.Seq[core.Primitive[T, T]],
) iter.Seq[core.Primitive[T, T]] {
	return func(yield func(core.Primitive[T, T]) bool) {
		for out := range Pipe(in, op.stages...) {
			if !yield(op.Carrier(out.Read())) {
				return
			}
		}
	}
}
