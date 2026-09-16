package calculus

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Minimum owns one field operation. Configuration supplies the value a run starts
from. What it hands over is the running minimum after every arrival, operating in-place
on the wire pointer.
*/
type Minimum struct {
	*core.PrimitiveError

	acc float64
}

func NewMinimum(current float64) *Minimum {
	return &Minimum{PrimitiveError: core.NewPrimitiveError(), acc: current}
}

func (minimum *Minimum) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			in := (*float64)(arriving)
			minimum.acc = math.Min(minimum.acc, *in)
			*in = minimum.acc

			if !yield(arriving) {
				return
			}
		}
	}
}
