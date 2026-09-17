package calculus

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Atanh owns one field operation. What it hands over is the inverse hyperbolic
tangent of each arrival.
*/
type Atanh struct {
	*core.PrimitiveError

	out float64
}

func NewAtanh() *Atanh {
	return &Atanh{PrimitiveError: core.NewPrimitiveError()}
}

func (atanh *Atanh) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			atanh.out = math.Atanh(*(*float64)(arriving))

			if !yield(unsafe.Pointer(&atanh.out)) {
				return
			}
		}
	}
}
