package calculus

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Sqrt owns one field operation. What it hands over is the square root of each
arrival, operating in-place on the wire pointer.
*/
type Sqrt struct {
	*core.PrimitiveError
}

func NewSqrt() *Sqrt {
	return &Sqrt{PrimitiveError: core.NewPrimitiveError()}
}

func (sqrt *Sqrt) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			in := (*float64)(arriving)
			*in = math.Sqrt(*in)

			if !yield(arriving) {
				return
			}
		}
	}
}
