package arithmetic

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Add owns one field operation. What arrives is already a pair: the left and
right operand as [2]float64. Each arrival maps independently, so the
primitive holds no state and owns no accumulation.
*/
type Add struct {
	*core.PrimitiveError

	out float64
}

/*
NewAdd creates the binary addition primitive.
*/
func NewAdd() *Add {
	return &Add{PrimitiveError: core.NewPrimitiveError()}
}

func (add *Add) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			pair := (*[2]float64)(arriving)
			add.out = pair[0] + pair[1]

			if !yield(unsafe.Pointer(&add.out)) {
				return
			}
		}
	}
}
