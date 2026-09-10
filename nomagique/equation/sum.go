package equation

import (
	"iter"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
Sum adds corresponding yields of two configured operations over one input run.
*/
type Sum[T any, U core.Numeric] struct {
	core.Base[T, U]
	left  core.Primitive[T, U]
	right core.Primitive[T, U]
}

func NewSum[T any, U core.Numeric](left, right core.Primitive[T, U]) *Sum[T, U] {
	return &Sum[T, U]{left: left, right: right}
}

func (op *Sum[T, U]) Next(
	in iter.Seq[core.Primitive[T, T]],
) iter.Seq[core.Primitive[U, U]] {
	return func(yield func(core.Primitive[U, U]) bool) {
		for pair := range transport.Zip(op.left.Next(in), op.right.Next(in)) {
			sides := pair.Read()

			if !yield(op.Carrier(sides.Left + sides.Right)) {
				return
			}
		}
	}
}
