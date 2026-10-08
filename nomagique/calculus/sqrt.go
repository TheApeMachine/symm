package calculus

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
Sqrt computes the square root of each arrival.
*/
type Sqrt struct {
	*core.PrimitiveError
}

func NewSqrt() core.Primitive {
	return &Sqrt{
		PrimitiveError: core.NewPrimitiveError(),
	}
}

func (op *Sqrt) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			val := *(*float64)(arriving)

			if val < 0 {
				op.Error(core.ErrShape)
				return
			}

			for value := range data.NewValue(math.Sqrt(val)).Next(nil) {
				if !yield(value) {
					return
				}
			}
		}
	}
}
