package calculus

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
RelativeChange owns (current - previous) / previous. A zero previous value is undefined.
*/
type RelativeChange struct {
	*core.PrimitiveError
	input  data.Map[string]
	output data.Map[float64]
}

func NewRelativeChange() *RelativeChange {
	output := data.NewOutputMap()
	output.Values["value"] = 0
	output.Values["relative_change"] = 0

	return &RelativeChange{
		PrimitiveError: core.NewPrimitiveError(),
		input: data.NewMap(
			"current", "current",
			"previous", "previous",
		),
		output: output,
	}
}

func (op *RelativeChange) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
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

			current, currentOK := values.Values["current"]
			previous, previousOK := values.Values["previous"]

			if !currentOK || !previousOK {
				op.Error(core.ErrNotHeld)
				return
			}

			if previous == 0 {
				op.Error(core.ErrDomain)
				return
			}

			result := (current - previous) / previous
			op.output.Values["value"] = result
			op.output.Values["relative_change"] = result

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
