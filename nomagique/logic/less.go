package logic

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Less owns one ordering relation. Pairing is external: what arrives is already
two values.
*/
type Less struct {
	*core.PrimitiveError

	out bool
}

func NewLess() *Less {
	return &Less{PrimitiveError: core.NewPrimitiveError()}
}

func (less *Less) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			in := (*[2]float64)(arriving)
			less.out = in[0] < in[1]

			if !yield(unsafe.Pointer(&less.out)) {
				return
			}
		}
	}
}
