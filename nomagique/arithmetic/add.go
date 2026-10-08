package arithmetic

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
Add owns one field operation, left + right. It reads the native operands "left"
and "right" through the arriving data.Adapter and publishes the result as
both "value" and "add". Each arrival maps independently, so the primitive
holds no state.
*/
type Add struct {
	*core.PrimitiveError
}

/*
NewAdd creates the binary addition primitive.
*/
func NewAdd() core.Primitive {
	return &Add{
		PrimitiveError: core.NewPrimitiveError(),
	}
}

func (op *Add) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		var values [2]float64
		idx := 0

		for arriving := range in {
			if idx < 2 {
				values[idx] = *(*float64)(arriving)
				idx++
			}
		}

		if idx < 2 {
			op.Error(errnie.Err(
				errnie.UnprocessableContent,
				"[arithmetic.add] insufficient inputs",
				nil,
			))

			return
		}

		for value := range data.NewValue(values[0] + values[1]).Next(nil) {
			if !yield(value) {
				return
			}
		}
	}
}
