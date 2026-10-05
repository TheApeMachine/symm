package calculus

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
SecondDifference owns 2*center - left - right.
*/
type SecondDifference struct {
	*core.PrimitiveError
	input  data.Map[string]
	output data.Map[float64]
}

func NewSecondDifference() *SecondDifference {
	output := data.NewOutputMap()
	output.Values["value"] = 0
	output.Values["second_difference"] = 0

	return &SecondDifference{
		PrimitiveError: core.NewPrimitiveError(),
		input: data.NewMap(
			"center", "center",
			"left", "left",
			"right", "right",
		),
		output: output,
	}
}

func (op *SecondDifference) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
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

			center, centerOK := values.Values["center"]
			left, leftOK := values.Values["left"]
			right, rightOK := values.Values["right"]

			if !centerOK || !leftOK || !rightOK {
				op.Error(core.ErrNotHeld)
				return
			}

			result := 2*center - left - right
			op.output.Values["value"] = result
			op.output.Values["second_difference"] = result

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
