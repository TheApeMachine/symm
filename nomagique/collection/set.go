package collection

import (
	"fmt"
	"iter"
	"slices"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Set replaces one indexed member without mutating its input collection.
*/
type Set[T any] struct {
	*core.PrimitiveError
	index int
	value T
	out   []T
}

func NewSet[T any](index int, value T) core.Primitive {
	return &Set[T]{
		PrimitiveError: core.NewPrimitiveError(),
		index:          index,
		value:          value,
	}
}

func (op *Set[T]) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			values := *(*[]T)(arriving)

			if op.index < 0 || op.index >= len(values) {
				op.Error(fmt.Errorf("%w: index %d of %d", core.ErrShape, op.index, len(values)))
				return
			}

			updated := slices.Clone(values)
			updated[op.index] = op.value
			op.out = updated

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}
