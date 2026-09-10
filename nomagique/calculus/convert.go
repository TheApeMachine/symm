package calculus

import (
	"iter"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Convert owns one representation change. What it hands over is each arrival
expressed as U.
*/
type Convert[A, B core.Numeric] struct {
	core.Base[A, B]
}

func NewConvert[A, B core.Numeric]() *Convert[A, B] {
	return &Convert[A, B]{}
}

func (op *Convert[A, B]) Next(
	in iter.Seq[core.Primitive[A, A]],
) iter.Seq[core.Primitive[B, B]] {
	return func(yield func(core.Primitive[B, B]) bool) {
		for arriving := range in {
			if !yield(op.Carrier(B(arriving.Read()))) {
				return
			}
		}
	}
}
