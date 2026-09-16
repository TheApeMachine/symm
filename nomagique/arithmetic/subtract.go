package arithmetic

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Subtract owns one field operation. What arrives is already a pair: the left
and right operand as [2]float64. Each arrival maps independently, so the
primitive holds no state.
*/
type Subtract struct {
	*core.PrimitiveError

	out float64
}

/*
NewSubtract creates the binary subtraction primitive.
*/
func NewSubtract() *Subtract {
	return &Subtract{PrimitiveError: core.NewPrimitiveError()}
}

func (subtract *Subtract) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			pair := (*[2]float64)(arriving)
			subtract.out = pair[0] - pair[1]

			if !yield(unsafe.Pointer(&subtract.out)) {
				return
			}
		}
	}
}
