package store

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Second is the trailing member of a pair, and the counterpart of First.
*/
type Second[T any] struct {
	*core.PrimitiveError

	out T
}

func NewSecond[T any]() *Second[T] {
	return &Second[T]{PrimitiveError: core.NewPrimitiveError()}
}

func (second *Second[T]) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			second.out = (*[2]T)(arriving)[1]

			if !yield(unsafe.Pointer(&second.out)) {
				return
			}
		}
	}
}
