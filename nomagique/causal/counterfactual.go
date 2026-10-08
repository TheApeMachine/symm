package causal

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
Counterfactual composes abduction, intervention and prediction.
*/
type Counterfactual struct {
	*core.PrimitiveError
	tolerance float64
}

func NewCounterfactual(tolerance float64) core.Primitive {
	return &Counterfactual{
		PrimitiveError: core.NewPrimitiveError(),
		tolerance:      tolerance,
	}
}

func (op *Counterfactual) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		var values [4]float64
		index := 0

		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			if index < 4 {
				values[index] = *(*float64)(arriving)
				index++
			}
		}

		if index < 4 {
			op.Error(core.ErrShape)
			return
		}

		actual := values[1]
		factual := values[2]
		predicted := values[3]

		noise := actual - factual
		absNoise := math.Abs(noise)
		counterfactual := predicted + noise
		precision := 1.0 / (1.0 + absNoise)
		defined := 1.0

		for value := range data.NewValue(counterfactual, noise, precision, defined).Next(nil) {
			if !yield(value) {
				return
			}
		}
	}
}
