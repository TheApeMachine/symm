package calculus

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
Absolute owns the absolute value transformation of an arrival.
*/
type Absolute struct {
	*core.PrimitiveError
}

func NewAbsolute() core.Primitive {
	return &Absolute{
		PrimitiveError: core.NewPrimitiveError(),
	}
}

func (op *Absolute) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			val := math.Abs(*(*float64)(arriving))

			for value := range data.NewValue(val).Next(nil) {
				if !yield(value) {
					return
				}
			}
		}
	}
}
