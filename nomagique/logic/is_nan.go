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
	return &IsNaN{
		PrimitiveError: core.NewPrimitiveError(),
	}
}

func (op *IsNaN) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			val := (*float64)(arriving)
			op.out = math.IsNaN(*val)

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}
