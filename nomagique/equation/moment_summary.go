package equation

import (
	"iter"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
MomentSummary projects Bessel-corrected sample variance from sufficient
statistics.
*/
type MomentSummary struct {
	core.Base[Moments, MomentReading]
}

func NewMomentSummary() *MomentSummary {
	return &MomentSummary{}
}

func (op *MomentSummary) Next(
	in iter.Seq[core.Primitive[Moments, Moments]],
) iter.Seq[core.Primitive[MomentReading, MomentReading]] {
	return func(yield func(core.Primitive[MomentReading, MomentReading]) bool) {
		for arriving := range in {
			var reading MomentReading
			reading.Summarize(arriving.Read())

			if !yield(op.Carrier(reading)) {
				return
			}
		}
	}
}
