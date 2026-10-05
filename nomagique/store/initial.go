package store

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
Initial retains the first defined value in a run and republishes that same
origin for every later observation.

Its native coordinates are:
value -> initial
value after the first defined observation -> subsequent.
*/
type Initial struct {
	*core.PrimitiveError
	seen   bool
	held   float64
	input  data.Map[string]
	output data.Map[float64]
}

func NewInitial() core.Primitive {
	output := data.NewOutputMap()
	output.Values["initial"] = 0

	return &Initial{
		PrimitiveError: core.NewPrimitiveError(),
		input:          data.NewMap("value", "value"),
		output:         output,
	}
}

func (op *Initial) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			adapter := *(**data.Adapter)(arriving)

			if adapter == nil {
				op.Error(core.ErrShape)
				return
			}

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

			delete(op.output.Values, "subsequent")

			if op.seen {
				op.output.Values["subsequent"] = value
			}

			if !op.seen {
				op.held = value
				op.seen = true
			}

			op.output.Values["initial"] = op.held

			for range adapter.Next(data.NewValue(op.output)) {
			}

			if !yield(arriving) {
				return
			}
		}
	}
}
