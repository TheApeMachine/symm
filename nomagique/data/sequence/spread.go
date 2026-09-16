package sequence

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Spread presents collection members as individual yields.
*/
type Spread[T any] struct {
	*core.PrimitiveError

	out T
}

func NewSpread[T any]() *Spread[T] {
	return &Spread[T]{PrimitiveError: core.NewPrimitiveError()}
}

func (spread *Spread[T]) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for collection := range in {
			slice := *(*[]T)(collection)

			for _, member := range slice {
				spread.out = member

				if !yield(unsafe.Pointer(&spread.out)) {
					return
				}
			}
		}
	}
}
