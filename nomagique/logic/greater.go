package logic

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Greater owns one ordering relation. Pairing is external: what arrives is already
two values.
*/
type Greater struct {
	*core.PrimitiveError

	out bool
}

func NewGreater() *Greater {
	return &Greater{PrimitiveError: core.NewPrimitiveError()}
}

func (greater *Greater) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			in := (*[2]float64)(arriving)
			greater.out = in[0] > in[1]

			if !yield(unsafe.Pointer(&greater.out)) {
				return
			}
		}
	}
}
