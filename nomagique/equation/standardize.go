package equation

import (
	"iter"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
StandardizeInput is a value and the center/scale that locate it.
*/
type StandardizeInput[U core.Floating] struct {
	Value  U
	Center U
	Scale  U
}

/*
Standardize owns (value - center) / scale. State and the choice of causal
center/scale are supplied in the arrival, not owned here.
*/
type Standardize[U core.Floating] struct {
	core.Base[StandardizeInput[U], U]
}

func NewStandardize[U core.Floating]() *Standardize[U] {
	return &Standardize[U]{}
}

func (op *Standardize[U]) Next(
	in iter.Seq[core.Primitive[StandardizeInput[U], StandardizeInput[U]]],
) iter.Seq[core.Primitive[U, U]] {
	return func(yield func(core.Primitive[U, U]) bool) {
		for arriving := range in {
			input := arriving.Read()

			if !yield(op.Carrier((input.Value - input.Center) / input.Scale)) {
				return
			}
		}
	}
}
