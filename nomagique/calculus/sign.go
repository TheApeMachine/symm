package calculus

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
Sign returns -1, 0, or 1 based on the sign of the arrival.
*/
type Sign struct {
	*core.PrimitiveError
}

func NewSign() core.Primitive {
	return &Sign{
		PrimitiveError: core.NewPrimitiveError(),
	}
}

func (op *Sign) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			val := *(*float64)(arriving)
			var sgn float64

			if val > 0 {
				sgn = 1.0
			}

			if val < 0 {
				sgn = -1.0
			}

			for value := range data.NewValue(sgn).Next(nil) {
				if !yield(value) {
					return
				}
			}
		}
	}
}
