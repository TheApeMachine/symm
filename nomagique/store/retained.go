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

func NewRetained[T any](current T) *Retained[T] {
	return &Retained[T]{PrimitiveError: core.NewPrimitiveError(), held: current}
}

func (retained *Retained[T]) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		if in == nil {
			yield(unsafe.Pointer(&retained.held))
			return
		}

		for arriving := range in {
			retained.held = *(*T)(arriving)

			if !yield(unsafe.Pointer(&retained.held)) {
				return
			}
		}
	}
}
