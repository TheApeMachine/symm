package store

import (
	"iter"
	"sync/atomic"
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

	held atomic.Pointer[T]
}

func NewRetained[T any](current ...T) *Retained[T] {
	retained := &Retained[T]{PrimitiveError: core.NewPrimitiveError()}

	if len(current) == 0 {
		return retained
	}

	value := current[0]
	retained.held.Store(&value)
	return retained
}

func (retained *Retained[T]) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		if in == nil {
			pointer := retained.held.Load()

			if pointer == nil {
				return
			}

			if !yield(unsafe.Pointer(pointer)) {
				return
			}

			return
		}

		wrote := false

		for arriving := range in {
			value := new(T)
			*value = *(*T)(arriving)
			retained.held.Store(value)
			wrote = true

			if !yield(unsafe.Pointer(value)) {
				return
			}
		}

		if wrote {
			return
		}

		pointer := retained.held.Load()

		if pointer == nil {
			return
		}

		yield(unsafe.Pointer(pointer))
	}
}

