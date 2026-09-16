package arithmetic

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Difference subtracts paired members. Unequal lengths are a shape error.
*/
type Difference struct {
	*core.PrimitiveError

	out []float64
}

func NewDifference() *Difference {
	return &Difference{PrimitiveError: core.NewPrimitiveError()}
}

func (difference *Difference) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			pair := (*Pair)(arriving)

			if len(pair.Left) != len(pair.Right) {
				difference.Error(core.ErrShape)
				continue
			}

			if len(difference.out) != len(pair.Left) {
				difference.out = make([]float64, len(pair.Left))
			}

			for index, value := range pair.Left {
				difference.out[index] = value - pair.Right[index]
			}

			if !yield(unsafe.Pointer(&difference.out)) {
				return
			}
		}
	}
}
