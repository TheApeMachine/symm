package sequence

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Append owns extending a collection.
*/
type Append[T any] struct {
	*core.PrimitiveError

	held []T
	out  []T
}

func NewAppend[T any](current []T) *Append[T] {
	return &Append[T]{PrimitiveError: core.NewPrimitiveError(), held: append([]T(nil), current...)}
}

func (appendPrimitive *Append[T]) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			appendPrimitive.held = append(appendPrimitive.held, *(*T)(arriving))
			appendPrimitive.out = appendPrimitive.held

			if !yield(unsafe.Pointer(&appendPrimitive.out)) {
				return
			}
		}
	}
}
