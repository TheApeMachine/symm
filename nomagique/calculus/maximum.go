package calculus

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Maximum owns one field operation. Configuration supplies the value a run starts
from. What it hands over is the running maximum after every arrival, operating in-place
on the wire pointer.
*/
type Maximum struct {
	*core.PrimitiveError

	acc float64
}

func NewMaximum(current float64) *Maximum {
	return &Maximum{PrimitiveError: core.NewPrimitiveError(), acc: current}
}

func (maximum *Maximum) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			in := (*float64)(arriving)
			maximum.acc = math.Max(maximum.acc, *in)
			*in = maximum.acc

			if !yield(arriving) {
				return
			}
		}
	}
}
