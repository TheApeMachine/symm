package equation

import (
	"iter"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
Ratio divides corresponding yields of two configured operations over one input
run.
*/
type Ratio[T any, U core.Floating] struct {
	core.Base[T, U]
	left  core.Primitive[T, U]
	right core.Primitive[T, U]
}

func NewRatio[T any, U core.Floating](left, right core.Primitive[T, U]) *Ratio[T, U] {
	return &Ratio[T, U]{left: left, right: right}
}

func (op *Ratio[T, U]) Next(
	in iter.Seq[core.Primitive[T, T]],
) iter.Seq[core.Primitive[U, U]] {
	return func(yield func(core.Primitive[U, U]) bool) {
		for pair := range transport.Zip(op.left.Next(in), op.right.Next(in)) {
			sides := pair.Read()

			if !yield(op.Carrier(sides.Left / sides.Right)) {
				return
			}
		}
	}
}
