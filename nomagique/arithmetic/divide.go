package arithmetic

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/* Divide owns one field division. A zero divisor leaves the quotient absent. */
type Divide struct {
	*core.PrimitiveError
	input  data.Map[string]
	output data.Map[float64]
}

func NewDivide() core.Primitive {
	output := data.NewOutputMap()
	output.Values["quotient"] = 0

	return &Divide{
		PrimitiveError: core.NewPrimitiveError(),
		input:          data.NewMap("left", "left", "right", "right"),
		output:         output,
	}
}

func (op *Divide) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			adapter := *(**data.Adapter)(arriving)
			var values data.Map[float64]

			for pointer := range adapter.Next(data.NewValue(op.input)) {
				values = *(*data.Map[float64])(pointer)
			}

			left, leftOK := values.Values["left"]
			right, rightOK := values.Values["right"]

			if !leftOK || !rightOK || right == 0 {
				if !yield(arriving) {
					return
				}
				continue
			}

			op.output.Values["quotient"] = left / right

			for range adapter.Next(data.NewValue(op.output)) {
			}

			if !yield(arriving) {
				return
			}
		}
	}
}
