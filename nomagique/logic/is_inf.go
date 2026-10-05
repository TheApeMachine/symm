package logic

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
IsInf owns the infinity predicate. It reports whether an arrival is infinite,
irrespective of sign.
*/
type IsInf struct {
	*core.PrimitiveError
	out bool
}

func NewIsInf() *IsInf {
	return &IsInf{
		PrimitiveError: core.NewPrimitiveError(),
	}
}

func (op *IsInf) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			val := (*float64)(arriving)
			op.out = math.IsInf(*val, 0)

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}
