package calculus

import (
	"iter"
	"math"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Absolute owns one field operation. What it hands over is the absolute value of
each arrival.
*/
type Absolute[U core.Floating] struct {
	core.Base[U, U]
}

func NewAbsolute[U core.Floating]() *Absolute[U] {
	return &Absolute[U]{}
}

func (op *Absolute[U]) Next(
	in iter.Seq[core.Primitive[U, U]],
) iter.Seq[core.Primitive[U, U]] {
	return func(yield func(core.Primitive[U, U]) bool) {
		for arriving := range in {
			if !yield(op.Carrier(U(math.Abs(float64(arriving.Read()))))) {
				return
			}
		}
	}
}
