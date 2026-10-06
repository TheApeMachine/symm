package arithmetic

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
Subtract owns one field operation, left - right. It reads the native operands "left"
and "right" through the arriving data.Adapter and publishes the result as
both "value" and "subtract". Each arrival maps independently, so the primitive
holds no state.
*/
type Subtract struct {
	*core.PrimitiveError
	input  data.Map[string]
	output data.Map[float64]
}

/*
NewSubtract creates the binary subtraction primitive.
*/
func NewSubtract() core.Primitive {
	output := data.NewOutputMap()
	output.Values["value"] = 0
	output.Values["subtract"] = 0

	return &Subtract{
		PrimitiveError: core.NewPrimitiveError(),
		input:          data.NewMap("left", "left", "right", "right"),
		output:         output,
	}
}

func (op *Subtract) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
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

			out := left - right
			op.output.Values["value"] = out
			op.output.Values["subtract"] = out

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
