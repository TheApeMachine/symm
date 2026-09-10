package logic

import (
	"iter"
	"math"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
IsInf owns the infinity predicate. It reports whether an arrival is infinite,
irrespective of sign.
*/
type IsInf[U core.Floating] struct {
	core.Base[U, bool]
}

func NewIsInf[U core.Floating]() *IsInf[U] {
	return &IsInf[U]{}
}

func (op *IsInf[U]) Next(
	in iter.Seq[core.Primitive[U, U]],
) iter.Seq[core.Primitive[bool, bool]] {
	return func(yield func(core.Primitive[bool, bool]) bool) {
		for arriving := range in {
			if !yield(op.Carrier(math.IsInf(float64(arriving.Read()), 0))) {
				return
			}
		}
	}
}
