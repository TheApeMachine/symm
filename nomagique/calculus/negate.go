package calculus

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
Negate computes -x for each arrival.
*/
type Negate struct {
	*core.PrimitiveError
}

func NewNegate() core.Primitive {
	return &Negate{
		PrimitiveError: core.NewPrimitiveError(),
	}
}

func (op *Negate) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			val := *(*float64)(arriving)

			for value := range data.NewValue(-val).Next(nil) {
				if !yield(value) {
					return
				}
			}
		}
	}
}
