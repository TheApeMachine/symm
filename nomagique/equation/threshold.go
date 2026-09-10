package equation

import (
	"iter"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
ThresholdInput is dispersion and the coefficient that scales it.
*/
type ThresholdInput[U core.Floating] struct {
	Dispersion  U
	Coefficient U
}

/*
Threshold multiplies dispersion by a coefficient. A source with no dispersion
has threshold one: there is nothing to scale.
*/
type Threshold[U core.Floating] struct {
	core.Base[ThresholdInput[U], U]
}

func NewThreshold[U core.Floating]() *Threshold[U] {
	return &Threshold[U]{}
}

func (op *Threshold[U]) Next(
	in iter.Seq[core.Primitive[ThresholdInput[U], ThresholdInput[U]]],
) iter.Seq[core.Primitive[U, U]] {
	return func(yield func(core.Primitive[U, U]) bool) {
		for arriving := range in {
			input := arriving.Read()
			value := U(1)

			if input.Dispersion > 0 {
				value = input.Dispersion * input.Coefficient
			}

			if !yield(op.Carrier(value)) {
				return
			}
		}
	}
}
