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
	input     data.Map[string]
	output    data.Map[float64]
}

func NewCounterfactual(tolerance float64) *Counterfactual {
	output := data.NewOutputMap()
	output.Values["counterfactual"] = 0
	output.Values["noise"] = 0
	output.Values["precision"] = 0
	output.Values["defined"] = 0

	return &Counterfactual{
		PrimitiveError: core.NewPrimitiveError(),
		tolerance:      tolerance,
		input: data.NewMap(
			"level", "level",
			"actual", "actual",
			"factual", "factual",
			"predicted", "predicted",
		),
		output: output,
	}
}

func (op *Counterfactual) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
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

			actual, actualOK := values.Values["actual"]
			factual, factualOK := values.Values["factual"]
			predicted, predictedOK := values.Values["predicted"]

			if !actualOK || !factualOK || !predictedOK {
				op.Error(core.ErrNotHeld)
				return
			}

			noise := actual - factual
			absNoise := math.Abs(noise)

			op.output.Values["noise"] = noise
			op.output.Values["counterfactual"] = predicted + noise
			op.output.Values["precision"] = 1.0 / (1.0 + absNoise)
			op.output.Values["defined"] = 1.0

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
