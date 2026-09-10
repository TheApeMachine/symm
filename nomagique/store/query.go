package store

import (
	"iter"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Query holds the latest arrival of a question. Selector-then-data ordering is
the caller's composition, not a second protocol inside Query.
*/
type QueryOp[T any] struct {
	core.Base[T, T]
}

func NewQuery[T any](current T) *QueryOp[T] {
	op := &QueryOp[T]{}
	op.Carrier(current)
	return op
}

/*
Query is a query with nothing in it yet: it takes what the run hands it.

A composition names its stages before it has values for them, so a retriever
placed mid-pipeline holds whatever reached it rather than something declared at
construction. Named as the type itself because that is what a stage in a
composition reads as: the stage is a Query, not a call that makes one.
*/
func Query[T any]() *QueryOp[T] {
	return &QueryOp[T]{}
}

/*
Next passes a run through, and produces one when there is none.

Query is where a composed pipeline starts: at the head there is nothing
upstream to range over, so it hands on what it holds. Anywhere else it is a
retained value the run flows through, taking the latest arrival as its own.
*/
func (op *QueryOp[T]) Next(
	in iter.Seq[core.Primitive[T, T]],
) iter.Seq[core.Primitive[T, T]] {
	return func(yield func(core.Primitive[T, T]) bool) {
		if in == nil {
			yield(op.Carrier(op.Read()))

			return
		}

		for arriving := range in {
			if !yield(op.Carrier(arriving.Read())) {
				return
			}
		}
	}
}
