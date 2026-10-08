package arithmetic

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
Multiply owns one field operation, left * right.
*/
type Multiply struct {
	*core.PrimitiveError
}

/*
NewMultiply creates the binary multiplication primitive.
*/
func NewMultiply() core.Primitive {
	return &Multiply{
		PrimitiveError: core.NewPrimitiveError(),
	}
}

func (op *Multiply) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
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

		for value := range data.NewValue(values[0] * values[1]).Next(nil) {
			if !yield(value) {
				return
			}
		}
	}
}
