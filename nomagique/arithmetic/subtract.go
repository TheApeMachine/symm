package arithmetic

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
Subtract owns one field operation, left - right.
*/
type Subtract struct {
	*core.PrimitiveError
}

/*
NewSubtract creates the binary subtraction primitive.
*/
func NewSubtract() core.Primitive {
	return &Subtract{
		PrimitiveError: core.NewPrimitiveError(),
	}
}

func (op *Subtract) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
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
			return
		}

		for value := range data.NewValue(values[0] - values[1]).Next(nil) {
			if !yield(value) {
				return
			}
		}
	}
}
