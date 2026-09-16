package calculus

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Sign owns one field operation. What it hands over is the unit sign of each
arrival, operating in-place on the wire pointer. Zero keeps its own value.
*/
type Sign struct {
	*core.PrimitiveError
}

func NewSign() *Sign {
	return &Sign{PrimitiveError: core.NewPrimitiveError()}
}

func (sign *Sign) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			in := (*float64)(arriving)
			if *in != 0 {
				*in = math.Copysign(1, *in)
			}

			if !yield(arriving) {
				return
			}
		}
	}
}
