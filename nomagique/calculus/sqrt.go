package calculus

import (
	"iter"
	"math"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Sqrt owns one field operation. What it hands over is the square root of each
arrival.
*/
type Sqrt[U core.Floating] struct {
	core.Base[U, U]
}

func NewSqrt[U core.Floating]() *Sqrt[U] {
	return &Sqrt[U]{}
}

func (op *Sqrt[U]) Next(
	in iter.Seq[core.Primitive[U, U]],
) iter.Seq[core.Primitive[U, U]] {
	return func(yield func(core.Primitive[U, U]) bool) {
		for arriving := range in {
			if !yield(op.Carrier(U(math.Sqrt(float64(arriving.Read()))))) {
				return
			}
		}
	}
}
