package calculus

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
SecondDifferenceInput is three neighbouring ordinates.
*/
type SecondDifferenceInput struct {
	Left   float64
	Center float64
	Right  float64
}

/*
SecondDifference owns 2*center - left - right.
*/
type SecondDifference struct {
	*core.PrimitiveError

	out float64
}

func NewSecondDifference() *SecondDifference {
	return &SecondDifference{PrimitiveError: core.NewPrimitiveError()}
}

func (secondDifference *SecondDifference) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			input := *(*SecondDifferenceInput)(arriving)
			secondDifference.out = input.Center + input.Center - input.Left - input.Right

			if !yield(unsafe.Pointer(&secondDifference.out)) {
				return
			}
		}
	}
}
