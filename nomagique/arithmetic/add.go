package arithmetic

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

// Add owns one binary field operation: left + right. Operands arrive separately.
type Add struct{ *core.PrimitiveError }

func NewAdd() core.Primitive { return &Add{PrimitiveError: core.NewPrimitiveError()} }

func (op *Add) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		var values [2]float64
		index := 0

		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			if index < 2 {
				values[index] = *(*float64)(arriving)
				index++
			}
		}

		if index < 2 {
			op.Error(core.ErrShape)
			return
		}

		for pointer := range data.NewValue(values[0] + values[1]).Next(nil) {
			if !yield(pointer) {
				return
			}
		}
	}
}
