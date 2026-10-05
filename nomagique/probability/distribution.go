package probability

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
Distribution owns confidence, ambiguity, and sharpness over probability readouts.
*/
type Distribution struct {
	*core.PrimitiveError
	input  data.Map[string]
	output data.Map[float64]
}

func NewDistribution() *Distribution {
	output := data.NewOutputMap()
	output.Values["confidence"] = 0
	output.Values["ambiguity"] = 0
	output.Values["sharpness"] = 0

	return &Distribution{
		PrimitiveError: core.NewPrimitiveError(),
		input: data.NewMap(
			"confidence", "confidence",
			"ambiguity", "ambiguity",
		),
		output: output,
	}
}

func (op *Distribution) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
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

			confidence, confOK := values.Values["confidence"]
			ambiguityVal, ambOK := values.Values["ambiguity"]

			if !confOK || !ambOK {
				op.Error(core.ErrNotHeld)
				return
			}

			op.output.Values["confidence"] = confidence
			op.output.Values["ambiguity"] = ambiguityVal
			op.output.Values["sharpness"] = 1.0 - ambiguityVal

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
