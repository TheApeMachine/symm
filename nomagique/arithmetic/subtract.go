package arithmetic

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/* Subtract owns one field subtraction. */
type Subtract struct {
	*core.PrimitiveError
	input  data.Map[string]
	output data.Map[float64]
}

func NewSubtract() core.Primitive {
	output := data.NewOutputMap()
	output.Values["difference"] = 0

	return &Subtract{
		PrimitiveError: core.NewPrimitiveError(),
		input:          data.NewMap("left", "left", "right", "right"),
		output:         output,
	}
}

func (op *Subtract) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			adapter := *(**data.Adapter)(arriving)
			var values data.Map[float64]

			for pointer := range adapter.Next(data.NewValue(op.input)) {
				values = *(*data.Map[float64])(pointer)
			}

			left, leftOK := values.Values["left"]
			right, rightOK := values.Values["right"]

			if !leftOK || !rightOK {
				if !yield(arriving) {
					return
				}
				continue
			}

			op.output.Values["difference"] = left - right

			for range adapter.Next(data.NewValue(op.output)) {
			}

			if !yield(arriving) {
				return
			}
		}
	}
}
