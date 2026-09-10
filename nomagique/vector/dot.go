package vector

import (
	"iter"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Dot owns the inner product of two equal-length vectors.
*/
type Dot struct {
	core.Base[Pair, float64]
}

func NewDot() *Dot {
	return &Dot{}
}

func (op *Dot) Next(
	in iter.Seq[core.Primitive[Pair, Pair]],
) iter.Seq[core.Primitive[float64, float64]] {
	return func(yield func(core.Primitive[float64, float64]) bool) {
		for arriving := range in {
			pair := arriving.Read()

			if len(pair.Left) != len(pair.Right) {
				op.Error(core.ErrShape)
				continue
			}

			var total float64

			for index, value := range pair.Left {
				total += value * pair.Right[index]
			}

			if !yield(op.Carrier(total)) {
				return
			}
		}
	}
}
