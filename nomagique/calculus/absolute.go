package calculus

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Absolute owns one field operation. What it hands over is the absolute value
of each arrival, operating in-place on the wire pointer.
*/
type Absolute struct {
	*core.PrimitiveError
}

func NewAbsolute() *Absolute {
	return &Absolute{PrimitiveError: core.NewPrimitiveError()}
}

func (absolute *Absolute) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			in := (*float64)(arriving)
			*in = math.Abs(*in)

			if !yield(arriving) {
				return
			}
		}
	}
}
