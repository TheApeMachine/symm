package equation

import (
	"iter"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
WeightedVariance owns E_w[x²] - E_w[x]². It is population dispersion, not an
unbiased sample-variance correction. Negative roundoff is reported as zero;
NaN remains undefined.
*/
type WeightedVariance[U core.Floating] struct {
	core.Base[Weighted[U], U]
	mass   U
	first  U
	second U
}

func NewWeightedVariance[U core.Floating]() *WeightedVariance[U] {
	return &WeightedVariance[U]{}
}

func (op *WeightedVariance[U]) Write(value Weighted[U]) {
	op.Base.Write(value)
	op.mass = 0
	op.first = 0
	op.second = 0
}

func (op *WeightedVariance[U]) Next(
	in iter.Seq[core.Primitive[Weighted[U], Weighted[U]]],
) iter.Seq[core.Primitive[U, U]] {
	return func(yield func(core.Primitive[U, U]) bool) {
		for arriving := range in {
			item := arriving.Read()
			op.mass += item.Weight
			op.first += item.Weight * item.Value
			op.second += item.Weight * item.Value * item.Value
			mean := op.first / op.mass
			value := op.second/op.mass - mean*mean

			if value < 0 {
				value = 0
			}

			if !yield(op.Carrier(value)) {
				return
			}
		}
	}
}
