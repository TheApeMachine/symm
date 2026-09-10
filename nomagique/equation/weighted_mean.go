package equation

import (
	"iter"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Weighted is one observation with its mass.
*/
type Weighted[U core.Floating] struct {
	Weight U
	Value  U
}

/*
WeightedMean owns sum(w x) / sum(w).
*/
type WeightedMean[U core.Floating] struct {
	core.Base[Weighted[U], U]
	mass  U
	total U
}

func NewWeightedMean[U core.Floating]() *WeightedMean[U] {
	return &WeightedMean[U]{}
}

func (op *WeightedMean[U]) Write(value Weighted[U]) {
	op.Base.Write(value)
	op.mass = 0
	op.total = 0
}

func (op *WeightedMean[U]) Next(
	in iter.Seq[core.Primitive[Weighted[U], Weighted[U]]],
) iter.Seq[core.Primitive[U, U]] {
	return func(yield func(core.Primitive[U, U]) bool) {
		for arriving := range in {
			item := arriving.Read()
			op.mass += item.Weight
			op.total += item.Weight * item.Value

			if !yield(op.Carrier(op.total / op.mass)) {
				return
			}
		}
	}
}
