package probability

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
Normalize divides an arriving value by a run total.
*/
type Normalize struct {
	*core.PrimitiveError
	input  data.Map[string]
	output data.Map[float64]
}

func NewNormalize() *Normalize {
	output := data.NewOutputMap()
	output.Values["normalized"] = 0

	return &Normalize{
		PrimitiveError: core.NewPrimitiveError(),
		input: data.NewMap(
			"value", "value",
			"total", "total",
		),
		output: output,
	}
}

func (op *Normalize) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
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
			total, totalOK := values.Values["total"]

			if !valOK || !totalOK {
				op.Error(core.ErrNotHeld)
				return
			}

			if total == 0 {
				op.Error(core.ErrDomain)
				return
			}

			op.output.Values["normalized"] = val / total

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
