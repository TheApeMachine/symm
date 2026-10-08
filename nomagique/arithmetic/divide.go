package arithmetic

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
Divide owns one field operation, left / right.
*/
type Divide struct {
	*core.PrimitiveError
}

/*
NewDivide creates the binary division primitive.
*/
func NewDivide() core.Primitive {
	return &Divide{
		PrimitiveError: core.NewPrimitiveError(),
	}
}

func (op *Divide) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
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

		res := 0.0
		if values[1] != 0 {
			res = values[0] / values[1]
		}

		for value := range data.NewValue(res).Next(nil) {
			if !yield(value) {
				return
			}
		}
	}
}
