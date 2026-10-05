package arithmetic

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
Midpoint owns the arithmetic midpoint of a strictly positive ordered interval.
An invalid interval leaves the midpoint undefined.
*/
type Midpoint struct {
	*core.PrimitiveError
	input  data.Map[string]
	output data.Map[float64]
}

func NewMidpoint() core.Primitive {
	output := data.NewOutputMap()
	output.Values["midpoint"] = 0

	return &Midpoint{
		PrimitiveError: core.NewPrimitiveError(),
		input:          data.NewMap("lower", "lower", "upper", "upper"),
		output:         output,
	}
}

func (op *Midpoint) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			adapter := *(**data.Adapter)(arriving)
			var values data.Map[float64]

			for pointer := range adapter.Next(data.NewValue(op.input)) {
				values = *(*data.Map[float64])(pointer)
			}

			lower, lowerOK := values.Values["lower"]
			upper, upperOK := values.Values["upper"]

			if !lowerOK || !upperOK || lower <= 0 || upper <= lower {
				if !yield(arriving) {
					return
				}
				continue
			}

			op.output.Values["midpoint"] = (lower + upper) / 2

			for range adapter.Next(data.NewValue(op.output)) {
			}

			if !yield(arriving) {
				return
			}
		}
	}
}
