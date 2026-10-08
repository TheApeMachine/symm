package calculus

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
Exp computes e^x for each arrival.
*/
type Exp struct {
	*core.PrimitiveError
}

func NewExp() core.Primitive {
	return &Exp{
		PrimitiveError: core.NewPrimitiveError(),
	}
}

func (op *Exp) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			for value := range data.NewValue(math.Exp(*(*float64)(arriving))).Next(nil) {
				if !yield(value) {
					return
				}
			}
		}
	}
}
