package calculus

import (
	"iter"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Reciprocal owns one field operation. What it hands over is the multiplicative
inverse of each arrival.
*/
type Reciprocal[U core.Floating] struct {
	core.Base[U, U]
}

func NewReciprocal[U core.Floating]() *Reciprocal[U] {
	return &Reciprocal[U]{}
}

func (op *Reciprocal[U]) Next(
	in iter.Seq[core.Primitive[U, U]],
) iter.Seq[core.Primitive[U, U]] {
	return func(yield func(core.Primitive[U, U]) bool) {
		for arriving := range in {
			if !yield(op.Carrier(1 / arriving.Read())) {
				return
			}
		}
	}
}
