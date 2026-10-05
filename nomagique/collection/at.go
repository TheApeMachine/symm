package collection

import (
	"fmt"
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
At selects an indexed member.
*/
type At[T any] struct {
	*core.PrimitiveError
	index int
	out   T
}

func NewAt[T any](index int) *At[T] {
	return &At[T]{
		PrimitiveError: core.NewPrimitiveError(),
		index:          index,
	}
}

func (op *At[T]) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			values := *(*[]T)(arriving)

			if op.index < 0 || op.index >= len(values) {
				op.Error(fmt.Errorf("%w: index %d of %d", core.ErrShape, op.index, len(values)))
				return
			}

			op.out = values[op.index]

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}
