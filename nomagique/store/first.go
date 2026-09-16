package store

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
First is the leading member of a pair. Pairs travel as one value, so taking
one member is the smallest operation that takes them apart.
*/
type First[T any] struct {
	*core.PrimitiveError

	out T
}

func NewFirst[T any]() *First[T] {
	return &First[T]{PrimitiveError: core.NewPrimitiveError()}
}

func (first *First[T]) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			first.out = (*[2]T)(arriving)[0]

			if !yield(unsafe.Pointer(&first.out)) {
				return
			}
		}
	}
}
