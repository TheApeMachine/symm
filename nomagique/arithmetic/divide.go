package arithmetic

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Divide owns one field operation. What arrives is already a pair: the dividend
and divisor as [2]float64. A zero divisor has no quotient, so the primitive
yields no fact rather than an infinity; undefined stays unwritten. Each
arrival maps independently, so the primitive holds no state.
*/
type Divide struct {
	*core.PrimitiveError

	out float64
}

/*
NewDivide creates the binary division primitive.
*/
func NewDivide() *Divide {
	return &Divide{PrimitiveError: core.NewPrimitiveError()}
}

func (divide *Divide) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			pair := (*[2]float64)(arriving)

			if pair[1] == 0 {
				return
			}

			divide.out = pair[0] / pair[1]

			if !yield(unsafe.Pointer(&divide.out)) {
				return
			}
		}
	}
}
