package store

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Constant replaces each arrival with a configured value. The arrival is the
clock; the payload is ignored.
*/
type Constant[T any] struct {
	*core.PrimitiveError

	out T
}

func NewConstant[T any](current T) *Constant[T] {
	return &Constant[T]{PrimitiveError: core.NewPrimitiveError(), out: current}
}

func (constant *Constant[T]) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for range in {
			if !yield(unsafe.Pointer(&constant.out)) {
				return
			}
		}
	}
}
