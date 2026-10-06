package transport

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Enumerate attaches a run-relative index to each value.
Each yield is *[2]any{index, value}.
*/
type Enumerate[T any] struct {
	*core.PrimitiveError
	out [2]any
}

func NewEnumerate[T any]() core.Primitive {
	return &Enumerate[T]{PrimitiveError: core.NewPrimitiveError()}
}

func (op *Enumerate[T]) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		index := 0

		for arriving := range in {
			op.out = [2]any{index, *(*T)(arriving)}

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}

			index++
		}
	}
}
