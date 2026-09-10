package calculus

import (
	"iter"
	"math"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Tanh owns one field operation. What it hands over is the hyperbolic tangent of
each arrival.
*/
type Tanh[U core.Floating] struct {
	core.Base[U, U]
}

func NewTanh[U core.Floating]() *Tanh[U] {
	return &Tanh[U]{}
}

func (op *Tanh[U]) Next(
	in iter.Seq[core.Primitive[U, U]],
) iter.Seq[core.Primitive[U, U]] {
	return func(yield func(core.Primitive[U, U]) bool) {
		for arriving := range in {
			if !yield(op.Carrier(U(math.Tanh(float64(arriving.Read()))))) {
				return
			}
		}
	}
}
