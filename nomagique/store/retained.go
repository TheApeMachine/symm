package store

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Retained holds the latest arrival. Configuration supplies the value before the
first update. An empty or nil run yields that held value without replacing it.
A value is yielded only after one has been written.
*/
type Retained[T any] struct {
	*core.PrimitiveError

	held    T
	written bool
}

func NewRetained[T any](current ...T) *Retained[T] {
	retained := &Retained[T]{PrimitiveError: core.NewPrimitiveError()}

	if len(current) == 0 {
		return retained
	}

	retained.held = current[0]
	retained.written = true
	return retained
}

func (retained *Retained[T]) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		if in == nil {
			if !retained.written {
				return
			}

			if !yield(unsafe.Pointer(&retained.held)) {
				return
			}

			return
		}

		wrote := false

		for arriving := range in {
			retained.held = *(*T)(arriving)
			retained.written = true
			wrote = true

			if !yield(unsafe.Pointer(&retained.held)) {
				return
			}
		}

		if wrote {
			return
		}

		if !retained.written {
			return
		}

		yield(unsafe.Pointer(&retained.held))
	}
}
