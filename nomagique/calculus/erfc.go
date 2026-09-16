package calculus

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Erfc owns one field operation. What it hands over is the complementary error
function of each arrival, operating in-place on the wire pointer.
*/
type Erfc struct {
	*core.PrimitiveError
}

func NewErfc() *Erfc {
	return &Erfc{PrimitiveError: core.NewPrimitiveError()}
}

func (erfc *Erfc) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			in := (*float64)(arriving)
			*in = math.Erfc(*in)

			if !yield(arriving) {
				return
			}
		}
	}
}
