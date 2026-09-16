package calculus

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Exp owns one field operation. What it hands over is the exponential of each
arrival, operating in-place on the wire pointer.
*/
type Exp struct {
	*core.PrimitiveError
}

func NewExp() *Exp {
	return &Exp{PrimitiveError: core.NewPrimitiveError()}
}

func (exp *Exp) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			in := (*float64)(arriving)
			*in = math.Exp(*in)

			if !yield(arriving) {
				return
			}
		}
	}
}
