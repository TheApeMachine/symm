package equation

import (
	"iter"
	"math"

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
	core.Base[SearchScaleInput, float64]
}

func NewSearchScale() *SearchScale {
	return &SearchScale{}
}

func (op *SearchScale) Next(
	in iter.Seq[core.Primitive[SearchScaleInput, SearchScaleInput]],
) iter.Seq[core.Primitive[float64, float64]] {
	return func(yield func(core.Primitive[float64, float64]) bool) {
		for arriving := range in {
			input := arriving.Read()

			if !yield(op.Carrier(math.Sqrt(2 * math.Log(input.Candidates) / input.Observations))) {
				return
			}
		}
	}
}
