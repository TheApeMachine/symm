package calculus

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
BoundRecord is a value and the interval that may replace it.
*/
type BoundRecord struct {
	Value float64
	Lower float64
	Upper float64
}

/*
Bound selects lower, value, or upper.
*/
type Bound struct {
	*core.PrimitiveError

	out float64
}

func NewBound() *Bound {
	return &Bound{PrimitiveError: core.NewPrimitiveError()}
}

func (bound *Bound) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			record := *(*BoundRecord)(arriving)
			val := record.Value

			if val < record.Lower {
				val = record.Lower
			}

			if val > record.Upper {
				val = record.Upper
			}

			bound.out = val

			if !yield(unsafe.Pointer(&bound.out)) {
				return
			}
		}
	}
}
