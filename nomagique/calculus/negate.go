package calculus

import (
	"iter"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Negate owns one field operation. What it hands over is the negation of each
arrival.
*/
type Negate[U core.Numeric] struct {
	core.Base[U, U]
}

func NewNegate[U core.Numeric]() *Negate[U] {
	return &Negate[U]{}
}

func (op *Negate[U]) Next(
	in iter.Seq[core.Primitive[U, U]],
) iter.Seq[core.Primitive[U, U]] {
	return func(yield func(core.Primitive[U, U]) bool) {
		for arriving := range in {
			if !yield(op.Carrier(-arriving.Read())) {
				return
			}
		}
	}
}
