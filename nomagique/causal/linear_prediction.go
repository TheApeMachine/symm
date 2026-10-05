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
	input  data.Map[string]
	output data.Map[float64]
}

func NewLinearPrediction() *LinearPrediction {
	output := data.NewOutputMap()
	output.Values["prediction"] = 0

	return &LinearPrediction{
		PrimitiveError: core.NewPrimitiveError(),
		input: data.NewMap(
			"intercept", "intercept",
			"weight", "weight",
			"feature", "feature",
		),
		output: output,
	}
}

func (op *LinearPrediction) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			adapter := *(**data.Adapter)(arriving)

			if adapter == nil {
				op.Error(core.ErrShape)
				return
			}

			var values data.Map[float64]

			for pointer := range adapter.Next(data.NewValue(op.input)) {
				values = *(*data.Map[float64])(pointer)
			}

			if err := adapter.Error(); err != nil {
				op.Error(err)
				return
			}

			intercept, interceptOK := values.Values["intercept"]
			weight, weightOK := values.Values["weight"]
			feature, featureOK := values.Values["feature"]

			if !interceptOK || !weightOK || !featureOK {
				op.Error(core.ErrNotHeld)
				return
			}

			op.output.Values["prediction"] = intercept + weight*feature

			for range adapter.Next(data.NewValue(op.output)) {
			}

			if err := adapter.Error(); err != nil {
				op.Error(err)
				return
			}

			if !yield(arriving) {
				return
			}
		}
	}
}
