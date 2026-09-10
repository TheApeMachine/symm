package calculus

import (
	"iter"
	"math"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Exp owns one field operation. What it hands over is e raised to each arrival.
*/
type Exp[U core.Floating] struct {
	core.Base[U, U]
}

func NewExp[U core.Floating]() *Exp[U] {
	return &Exp[U]{}
}

func (op *Exp[U]) Next(
	in iter.Seq[core.Primitive[U, U]],
) iter.Seq[core.Primitive[U, U]] {
	return func(yield func(core.Primitive[U, U]) bool) {
		for arriving := range in {
			if !yield(op.Carrier(U(math.Exp(float64(arriving.Read()))))) {
				return
			}
		}
	}
}
