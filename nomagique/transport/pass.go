package transport

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Pass forwards an arriving run unchanged.
*/
type Pass struct {
	*core.PrimitiveError
}

func NewPass() core.Primitive {
	return &Pass{PrimitiveError: core.NewPrimitiveError()}
}

func (op *Pass) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		if in == nil {
			return
		}

		for arriving := range in {
			if !yield(arriving) {
				return
			}
		}
	}
}
