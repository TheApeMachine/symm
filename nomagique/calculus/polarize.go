package calculus

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
Polarize splits a signed value into nonnegative components and normalizes
against a configured scale.
*/
type Polarize struct {
	*core.PrimitiveError
	input  data.Map[string]
	output data.Map[float64]
}

func NewPolarize() *Polarize {
	output := data.NewOutputMap()
	output.Values["alpha"] = 0
	output.Values["beta"] = 0
	output.Values["scale"] = 0
	output.Values["alpha_normalized"] = 0
	output.Values["beta_normalized"] = 0
	output.Values["value"] = 0

	return &Polarize{
		PrimitiveError: core.NewPrimitiveError(),
		input: data.NewMap(
			"value", "value",
			"scale", "scale",
		),
		output: output,
	}
}

func (op *Polarize) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
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

			val, valOK := values.Values["value"]
			scale, scaleOK := values.Values["scale"]

			if !valOK || !scaleOK {
				op.Error(core.ErrNotHeld)
				return
			}

			alpha := val
			beta := -val

			if alpha < 0 {
				alpha = 0
			}

			if beta < 0 {
				beta = 0
			}

			alphaNormalized := 0.0
			betaNormalized := 0.0

			if scale > 0 {
				alphaNormalized = alpha / (alpha + scale)
				betaNormalized = beta / (beta + scale)
			}

			normalizedValue := alphaNormalized - betaNormalized

			op.output.Values["alpha"] = alpha
			op.output.Values["beta"] = beta
			op.output.Values["scale"] = scale
			op.output.Values["alpha_normalized"] = alphaNormalized
			op.output.Values["beta_normalized"] = betaNormalized
			op.output.Values["value"] = normalizedValue

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
