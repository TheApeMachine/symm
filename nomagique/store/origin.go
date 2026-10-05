package store

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/* Origin retains and republishes the first observed numeric coordinate. */
type Origin struct {
	*core.PrimitiveError
	seen   bool
	held   float64
	input  data.Map[string]
	output data.Map[float64]
}

func NewOrigin() core.Primitive {
	output := data.NewOutputMap()
	output.Values["origin"] = 0

	return &Origin{
		PrimitiveError: core.NewPrimitiveError(),
		input:          data.NewMap("value", "value"),
		output:         output,
	}
}

func (op *Origin) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			adapter := *(**data.Adapter)(arriving)
			var values data.Map[float64]

			for pointer := range adapter.Next(data.NewValue(op.input)) {
				values = *(*data.Map[float64])(pointer)
			}

			value, ok := values.Values["value"]

			if !ok {
				if !yield(arriving) {
					return
				}
				continue
			}

			if !op.seen {
				op.held = value
				op.seen = true
			}

			op.output.Values["origin"] = op.held

			for range adapter.Next(data.NewValue(op.output)) {
			}

			if !yield(arriving) {
				return
			}
		}
	}
}
