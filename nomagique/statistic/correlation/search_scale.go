package correlation

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
SearchScaleInput is the candidate count and the observation count that scale
a lead/lag threshold.
*/
type SearchScaleInput struct {
	Candidates   float64
	Observations float64
}

/*
SearchScale owns sqrt(2 log(candidates) / observations).
*/
type SearchScale struct {
	*core.PrimitiveError
}

func NewSearchScale() *SearchScale {
	return &SearchScale{PrimitiveError: core.NewPrimitiveError()}
}

func (searchScale *SearchScale) Next(
	in iter.Seq[unsafe.Pointer],
) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			input := (*SearchScaleInput)(arriving)
			out := math.Sqrt((core.Unit + core.Unit) * math.Log(input.Candidates) / input.Observations)

			if !yield(unsafe.Pointer(&out)) {
				return
			}
		}
	}
}
