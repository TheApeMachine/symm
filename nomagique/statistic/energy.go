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
	return &Energy{PrimitiveError: core.NewPrimitiveError()}
}

func (energy *Energy) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			val := *(*float64)(arriving)
			energy.acc += val * val
			energy.out = energy.acc

			if !yield(unsafe.Pointer(&energy.out)) {
				return
			}
		}
	}
}
