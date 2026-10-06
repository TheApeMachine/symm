package arithmetic

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
Divide owns one field operation, left / right. It reads the native operands "left"
and "right" through the arriving data.Adapter and publishes the result as
both "value" and "divide". A zero divisor has no quotient, so the primitive
publishes nothing rather than an infinity; undefined stays unwritten and the
arrival is handed on. Each arrival maps independently, so the primitive holds
no state.
*/
type Divide struct {
	*core.PrimitiveError
	input  data.Map[string]
	output data.Map[float64]
}

/*
NewDivide creates the binary division primitive.
*/
func NewDivide() core.Primitive {
	output := data.NewOutputMap()
	output.Values["value"] = 0
	output.Values["divide"] = 0

	return &Divide{
		PrimitiveError: core.NewPrimitiveError(),
		input:          data.NewMap("left", "left", "right", "right"),
		output:         output,
	}
}

func (op *Divide) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
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

			left, leftOK := values.Values["left"]
			right, rightOK := values.Values["right"]

			if !leftOK || !rightOK {
				op.Error(core.ErrNotHeld)
				return
			}

			if right == 0 {
				if !yield(arriving) {
					return
				}

				continue
			}

			out := left / right
			op.output.Values["value"] = out
			op.output.Values["divide"] = out

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
