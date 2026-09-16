package calculus

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Square owns one field operation. What it hands over is the square of each
arrival, operating in-place on the wire pointer.
*/
type Square struct {
	*core.PrimitiveError
}

func NewSquare() *Square {
	return &Square{PrimitiveError: core.NewPrimitiveError()}
}

func (square *Square) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			in := (*float64)(arriving)
			*in = *in * *in

			if !yield(arriving) {
				return
			}
		}
	}
}
