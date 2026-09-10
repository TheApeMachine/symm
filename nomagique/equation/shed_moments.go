package equation

import (
	"iter"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
ShedMoments applies support-shedding without changing mean or sample
dispersion. Non-contraction requests leave the statistics unchanged.
*/
type ShedMoments struct {
	core.Base[MomentRetentionInput, Moments]
}

func NewShedMoments() *ShedMoments {
	return &ShedMoments{}
}

func (op *ShedMoments) Next(
	in iter.Seq[core.Primitive[MomentRetentionInput, MomentRetentionInput]],
) iter.Seq[core.Primitive[Moments, Moments]] {
	return func(yield func(core.Primitive[Moments, Moments]) bool) {
		for arriving := range in {
			input := arriving.Read()
			moments := input.Moments
			moments.Shed(input.Retain)

			if !yield(op.Carrier(moments)) {
				return
			}
		}
	}
}
