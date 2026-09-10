package calculus

import (
	"iter"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Square owns one field operation. What it hands over is the square of each
arrival.
*/
type Square[U core.Numeric] struct {
	core.Base[U, U]
}

func NewSquare[U core.Numeric]() *Square[U] {
	return &Square[U]{}
}

func (op *Square[U]) Next(
	in iter.Seq[core.Primitive[U, U]],
) iter.Seq[core.Primitive[U, U]] {
	return func(yield func(core.Primitive[U, U]) bool) {
		for arriving := range in {
			value := arriving.Read()

			if !yield(op.Carrier(value * value)) {
				return
			}
		}
	}
}
