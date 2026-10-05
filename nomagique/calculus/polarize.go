package calculus

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/* Polarize splits one signed coordinate into nonnegative components. */
type Polarize struct {
	*core.PrimitiveError
	input  data.Map[string]
	output data.Map[float64]
}

func NewPolarize() core.Primitive {
	output := data.NewOutputMap()
	output.Values["positive"] = 0
	output.Values["negative"] = 0

	return &Polarize{
		PrimitiveError: core.NewPrimitiveError(),
		input:          data.NewMap("value", "value"),
		output:         output,
	}
}

func (op *Polarize) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
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

			positive := value
			negative := -value

			if positive < 0 {
				positive = 0
			}

			if negative < 0 {
				negative = 0
			}

			op.output.Values["positive"] = positive
			op.output.Values["negative"] = negative

			for range adapter.Next(data.NewValue(op.output)) {
			}

			if !yield(arriving) {
				return
			}
		}
	}
}
