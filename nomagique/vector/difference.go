package vector

import (
	"iter"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Difference subtracts paired members. Unequal lengths are a shape error.
*/
type Difference struct {
	core.Base[Pair, []float64]
}

func NewDifference() *Difference {
	return &Difference{}
}

func (op *Difference) Next(
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
				out[index] = value - pair.Right[index]
			}

			if !yield(op.Carrier(out)) {
				return
			}
		}
	}
}
