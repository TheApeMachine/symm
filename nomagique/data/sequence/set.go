package sequence

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

func NewSet[T any](index int, value T) *Set[T] {
	return &Set[T]{PrimitiveError: core.NewPrimitiveError(), index: index, value: value}
}

func (set *Set[T]) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			values := *(*[]T)(arriving)

			if set.index < 0 || set.index >= len(values) {
				set.Error(fmt.Errorf("%w: index %d of %d", core.ErrShape, set.index, len(values)))
				return
			}

			updated := slices.Clone(values)
			updated[set.index] = set.value
			set.out = updated

			if !yield(unsafe.Pointer(&set.out)) {
				return
			}
		}
	}
}
