package equation

import (
	"iter"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
MomentRetentionInput is sufficient statistics and the mass ratio to keep.
*/
type MomentRetentionInput struct {
	Moments Moments
	Retain  float64
}

/*
MomentRetention changes sample mass while preserving corrected dispersion.
*/
type MomentRetention struct {
	core.Base[MomentRetentionInput, Moments]
}

func NewMomentRetention() *MomentRetention {
	return &MomentRetention{}
}

func (op *MomentRetention) Next(
	in iter.Seq[core.Primitive[MomentRetentionInput, MomentRetentionInput]],
) iter.Seq[core.Primitive[Moments, Moments]] {
	return func(yield func(core.Primitive[Moments, Moments]) bool) {
		for arriving := range in {
			input := arriving.Read()
			moments := input.Moments
			moments.Retain(input.Retain)

			if !yield(op.Carrier(moments)) {
				return
			}
		}
	}
}
