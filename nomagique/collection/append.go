package collection

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
	return &Append[T]{
		PrimitiveError: core.NewPrimitiveError(),
		held:           append([]T(nil), current...),
	}
}

func (op *Append[T]) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			op.held = append(op.held, *(*T)(arriving))
			op.out = op.held

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}
