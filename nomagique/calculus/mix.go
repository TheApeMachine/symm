package calculus

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
Mix owns left + weight*(right-left). Zero weight selects left; one selects right.
*/
type Mix struct {
	*core.PrimitiveError
	input  data.Map[string]
	output data.Map[float64]
}

func NewMix() *Mix {
	output := data.NewOutputMap()
	output.Values["value"] = 0
	output.Values["mix"] = 0

	return &Mix{
		PrimitiveError: core.NewPrimitiveError(),
		input: data.NewMap(
			"left", "left",
			"right", "right",
			"weight", "weight",
		),
		output: output,
	}
}

func (op *Mix) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
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

			left, leftOK := values.Values["left"]
			right, rightOK := values.Values["right"]
			weight, weightOK := values.Values["weight"]

			if !leftOK || !rightOK || !weightOK {
				op.Error(core.ErrNotHeld)
				return
			}

			result := left + weight*(right-left)
			op.output.Values["value"] = result
			op.output.Values["mix"] = result

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
