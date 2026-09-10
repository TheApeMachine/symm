package equation

import (
	"iter"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
SecondDifferenceInput is three neighbouring ordinates.
*/
type SecondDifferenceInput[U core.Numeric] struct {
	Left   U
	Center U
	Right  U
}

/*
SecondDifference owns 2*center - left - right.
*/
type SecondDifference[U core.Numeric] struct {
	core.Base[SecondDifferenceInput[U], U]
}

func NewSecondDifference[U core.Numeric]() *SecondDifference[U] {
	return &SecondDifference[U]{}
}

func (op *SecondDifference[U]) Next(
	in iter.Seq[core.Primitive[SecondDifferenceInput[U], SecondDifferenceInput[U]]],
) iter.Seq[core.Primitive[U, U]] {
	return func(yield func(core.Primitive[U, U]) bool) {
		for arriving := range in {
			input := arriving.Read()

			if !yield(op.Carrier(input.Center + input.Center - input.Left - input.Right)) {
				return
			}
		}
	}
}
