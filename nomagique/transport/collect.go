package transport

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Collect retains the values of one run as one collection.
*/
type Collect[T any] struct {
	*core.PrimitiveError
	out []T
}

func NewCollect[T any]() core.Primitive {
	return &Collect[T]{PrimitiveError: core.NewPrimitiveError()}
}

func (op *Collect[T]) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		var gathered []T

		for arriving := range in {
			gathered = append(gathered, *(*T)(arriving))
		}

		op.out = gathered

		if !yield(unsafe.Pointer(&op.out)) {
			return
		}
	}
}
