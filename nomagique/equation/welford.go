package equation

import (
	"iter"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Welford owns streaming sufficient statistics. Each arrival updates the moments
and hands over the before/after reading.
*/
type Welford struct {
	core.Base[float64, MomentReading]
	moments Moments
}

func NewWelford() *Welford {
	return &Welford{}
}

func (op *Welford) Next(
	in iter.Seq[core.Primitive[float64, float64]],
) iter.Seq[core.Primitive[MomentReading, MomentReading]] {
	return func(yield func(core.Primitive[MomentReading, MomentReading]) bool) {
		for arriving := range in {
			reading := op.moments.Update(arriving.Read())

			if !yield(op.Carrier(reading)) {
				return
			}
		}
	}
}
