package calculus

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
Square computes x * x for each arrival.
*/
type Square struct {
	*core.PrimitiveError
}

func NewSquare() core.Primitive {
	return &Square{
		PrimitiveError: core.NewPrimitiveError(),
	}
}

func (op *Square) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			val := *(*float64)(arriving)

			for value := range data.NewValue(val * val).Next(nil) {
				if !yield(value) {
					return
				}
			}
		}
	}
}
