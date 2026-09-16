package logic

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
IsNaN owns the undefinedness predicate. It reports whether an arrival is NaN;
it does not replace, skip, or otherwise keep invalid state alive.
*/
type IsNaN struct {
	*core.PrimitiveError

	out bool
}

func NewIsNaN() *IsNaN {
	return &IsNaN{PrimitiveError: core.NewPrimitiveError()}
}

func (isNaN *IsNaN) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			in := (*float64)(arriving)
			isNaN.out = math.IsNaN(*in)

			if !yield(unsafe.Pointer(&isNaN.out)) {
				return
			}
		}
	}
}
