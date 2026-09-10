package logic

import (
	"iter"
	"math"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
IsNaN owns the undefinedness predicate. It reports whether an arrival is NaN;
it does not replace, skip, or otherwise keep invalid state alive.
*/
type IsNaN[U core.Floating] struct {
	core.Base[U, bool]
}

func NewIsNaN[U core.Floating]() *IsNaN[U] {
	return &IsNaN[U]{}
}

func (op *IsNaN[U]) Next(
	in iter.Seq[core.Primitive[U, U]],
) iter.Seq[core.Primitive[bool, bool]] {
	return func(yield func(core.Primitive[bool, bool]) bool) {
		for arriving := range in {
			if !yield(op.Carrier(math.IsNaN(float64(arriving.Read())))) {
				return
			}
		}
	}
}
