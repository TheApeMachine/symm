package sequence

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Indexed is a value and where it fell in its run.
*/
type Indexed[T any] struct {
	Index int
	Value T
}

/*
Enumerate attaches a run-relative index to each value.
*/
type Enumerate[T any] struct {
	*core.PrimitiveError

	out Indexed[T]
}

func NewEnumerate[T any]() *Enumerate[T] {
	return &Enumerate[T]{PrimitiveError: core.NewPrimitiveError()}
}

func (enumerate *Enumerate[T]) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		index := 0

		for arriving := range in {
			enumerate.out = Indexed[T]{Index: index, Value: *(*T)(arriving)}

			if !yield(unsafe.Pointer(&enumerate.out)) {
				return
			}

			index++
		}
	}
}
