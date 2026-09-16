package calculus

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Negate owns one field operation. What it hands over is the additive inverse
of each arrival, operating in-place on the wire pointer.
*/
type Negate struct {
	*core.PrimitiveError
}

func NewNegate() *Negate {
	return &Negate{PrimitiveError: core.NewPrimitiveError()}
}

func (negate *Negate) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			in := (*float64)(arriving)
			*in = -*in

			if !yield(arriving) {
				return
			}
		}
	}
}
