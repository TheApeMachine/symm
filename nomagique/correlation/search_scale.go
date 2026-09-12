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
	err error
	out float64
}

func NewSearchScale() core.Primitive {
	return &SearchScale{}
}

func (op *SearchScale) Next(
	in iter.Seq[unsafe.Pointer],
) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			input := (*SearchScaleInput)(arriving)
			op.out = math.Sqrt(2.0 * math.Log(input.Candidates) / input.Observations)

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

func (op *SearchScale) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = err
			break
		}
	}

	return op.err
}
