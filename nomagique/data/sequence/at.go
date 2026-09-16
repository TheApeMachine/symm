package sequence

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
	return &At[T]{PrimitiveError: core.NewPrimitiveError(), index: index}
}

func (at *At[T]) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			values := *(*[]T)(arriving)

			if at.index < 0 || at.index >= len(values) {
				at.Error(fmt.Errorf("%w: index %d of %d", core.ErrShape, at.index, len(values)))
				return
			}

			at.out = values[at.index]

			if !yield(unsafe.Pointer(&at.out)) {
				return
			}
		}
	}
}
