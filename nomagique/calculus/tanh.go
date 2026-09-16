package calculus

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Tanh owns one field operation. What it hands over is the hyperbolic tangent
of each arrival, operating in-place on the wire pointer.
*/
type Tanh struct {
	*core.PrimitiveError
}

func NewTanh() *Tanh {
	return &Tanh{PrimitiveError: core.NewPrimitiveError()}
}

func (tanh *Tanh) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			in := (*float64)(arriving)
			*in = math.Tanh(*in)

			if !yield(arriving) {
				return
			}
		}
	}
}
