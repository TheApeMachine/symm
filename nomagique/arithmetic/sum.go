package arithmetic

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Pair is two vectors of equal length.
*/
type Pair struct {
	Left  []float64
	Right []float64
}

/*
Sum adds paired members. Unequal lengths are a shape error.
*/
type Sum struct {
	*core.PrimitiveError

	out []float64
}

func NewSum() *Sum {
	return &Sum{PrimitiveError: core.NewPrimitiveError()}
}

func (sum *Sum) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			pair := (*Pair)(arriving)

			if len(pair.Left) != len(pair.Right) {
				sum.Error(core.ErrShape)
				continue
			}

			if len(sum.out) != len(pair.Left) {
				sum.out = make([]float64, len(pair.Left))
			}

			for index, value := range pair.Left {
				sum.out[index] = value + pair.Right[index]
			}

			if !yield(unsafe.Pointer(&sum.out)) {
				return
			}
		}
	}
}
