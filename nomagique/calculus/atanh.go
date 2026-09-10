package calculus

import (
	"iter"
	"math"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Atanh owns one field operation. What it hands over is the inverse hyperbolic
tangent of each arrival.
*/
type Atanh[U core.Floating] struct {
	core.Base[U, U]
}

func NewAtanh[U core.Floating]() *Atanh[U] {
	return &Atanh[U]{}
}

func (op *Atanh[U]) Next(
	in iter.Seq[core.Primitive[U, U]],
) iter.Seq[core.Primitive[U, U]] {
	return func(yield func(core.Primitive[U, U]) bool) {
		for arriving := range in {
			if !yield(op.Carrier(U(math.Atanh(float64(arriving.Read()))))) {
				return
			}
		}
	}
}
