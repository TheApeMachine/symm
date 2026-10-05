package statistic

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Energy owns the sum of squares of each arrival.
*/
type Energy struct {
	*core.PrimitiveError
	acc float64
	out float64
}

func NewEnergy() *Energy {
	return &Energy{
		PrimitiveError: core.NewPrimitiveError(),
	}
}

func (op *Energy) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			val := *(*float64)(arriving)
			op.acc += val * val
			op.out = op.acc

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}
