package causal

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
LinearPrediction evaluates linear combination of inputs and weights.
*/
type LinearPrediction struct {
	*core.PrimitiveError
}

func NewLinearPrediction() core.Primitive {
	return &LinearPrediction{
		PrimitiveError: core.NewPrimitiveError(),
	}
}

func (op *LinearPrediction) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		var values [3]float64
		index := 0

		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			if index < 3 {
				values[index] = *(*float64)(arriving)
				index++
			}
		}

		if index < 3 {
			op.Error(core.ErrShape)
			return
		}

		intercept := values[0]
		weight := values[1]
		feature := values[2]

		prediction := intercept + weight*feature

		for value := range data.NewValue(prediction).Next(nil) {
			if !yield(value) {
				return
			}
		}
	}
}
