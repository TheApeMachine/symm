package equation

import (
	"iter"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
MomentUpdateInput is the current sufficient statistics and the arriving value.
*/
type MomentUpdateInput struct {
	Moments Moments
	Value   float64
}

/*
MomentUpdate applies the canonical typed recurrence.
*/
type MomentUpdate struct {
	core.Base[MomentUpdateInput, MomentReading]
}

func NewMomentUpdate() *MomentUpdate {
	return &MomentUpdate{}
}

func (op *MomentUpdate) Next(
	in iter.Seq[core.Primitive[MomentUpdateInput, MomentUpdateInput]],
) iter.Seq[core.Primitive[MomentReading, MomentReading]] {
	return func(yield func(core.Primitive[MomentReading, MomentReading]) bool) {
		for arriving := range in {
			input := arriving.Read()
			moments := input.Moments
			reading := moments.Update(input.Value)

			if !yield(op.Carrier(reading)) {
				return
			}
		}
	}
}
