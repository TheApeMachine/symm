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
	return &IsInf{PrimitiveError: core.NewPrimitiveError()}
}

func (isInf *IsInf) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			in := (*float64)(arriving)
			isInf.out = math.IsInf(*in, 0)

			if !yield(unsafe.Pointer(&isInf.out)) {
				return
			}
		}
	}
}
