package calculus

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
Minimum tracks the running minimum after every arrival.
*/
type Minimum struct {
	*core.PrimitiveError
}

func NewMinimum() core.Primitive {
	return &Minimum{
		PrimitiveError: core.NewPrimitiveError(),
	}
}

func (op *Minimum) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		seen := false
		minVal := math.MaxFloat64

		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			val := *(*float64)(arriving)
			if !seen || val < minVal {
				minVal = val
				seen = true
			}
		}

		if !seen {
			return
		}

		for value := range data.NewValue(minVal).Next(nil) {
			if !yield(value) {
				return
			}
		}
	}
}
