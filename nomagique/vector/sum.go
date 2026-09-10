package vector

import (
	"iter"

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
	core.Base[Pair, []float64]
}

func NewSum() *Sum {
	return &Sum{}
}

func (op *Sum) Next(
	in iter.Seq[core.Primitive[Pair, Pair]],
) iter.Seq[core.Primitive[[]float64, []float64]] {
	return func(yield func(core.Primitive[[]float64, []float64]) bool) {
		for arriving := range in {
			pair := arriving.Read()

			if len(pair.Left) != len(pair.Right) {
				op.Error(core.ErrShape)
				continue
			}

			out := make([]float64, len(pair.Left))

			for index, value := range pair.Left {
				out[index] = value + pair.Right[index]
			}

			if !yield(op.Carrier(out)) {
				return
			}
		}
	}
}
