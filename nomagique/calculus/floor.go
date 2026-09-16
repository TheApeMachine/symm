package calculus

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Floor owns one field operation. What it hands over is the floor of each
arrival, operating in-place on the wire pointer.
*/
type Floor struct {
	*core.PrimitiveError
}

func NewFloor() *Floor {
	return &Floor{PrimitiveError: core.NewPrimitiveError()}
}

func (floor *Floor) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			in := (*float64)(arriving)
			*in = math.Floor(*in)

			if !yield(arriving) {
				return
			}
		}
	}
}
