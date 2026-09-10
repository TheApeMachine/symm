package logic

import (
	"iter"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Reject owns explicit refusal. It records the configured reason, consumes the
run, and hands nothing over.
*/
type Reject[T any] struct {
	core.Base[T, T]
	reason error
}

func NewReject[T any](reason error) *Reject[T] {
	return &Reject[T]{reason: reason}
}

func (op *Reject[T]) Next(
	in iter.Seq[core.Primitive[T, T]],
) iter.Seq[core.Primitive[T, T]] {
	return func(func(core.Primitive[T, T]) bool) {
		op.Error(op.reason)

		for range in {
		}
	}
}
