package store

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Retained holds the latest arrival. Configuration supplies the value before the
first update.
*/
type Retained[T any] struct {
	*core.PrimitiveError
	held T
}

func NewRetained[T any](current T) core.Primitive {
	return &Retained[T]{
		PrimitiveError: core.NewPrimitiveError(),
		held:           current,
	}
}

func (op *Retained[T]) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		if in == nil {
			yield(unsafe.Pointer(&op.held))
			return
		}

		for arriving := range in {
			op.held = *(*T)(arriving)

			if !yield(unsafe.Pointer(&op.held)) {
				return
			}
		}
	}
}
