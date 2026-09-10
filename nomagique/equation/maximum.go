package equation

import (
	"iter"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
Maximum selects the larger of two configured operations over one input run.
*/
type Maximum[T any, U core.Numeric] struct {
	core.Base[T, U]
	left  core.Primitive[T, U]
	right core.Primitive[T, U]
}

func NewMaximum[T any, U core.Numeric](left, right core.Primitive[T, U]) *Maximum[T, U] {
	return &Maximum[T, U]{left: left, right: right}
}

func (op *Maximum[T, U]) Next(
	in iter.Seq[core.Primitive[T, T]],
) iter.Seq[core.Primitive[U, U]] {
	return func(yield func(core.Primitive[U, U]) bool) {
		for pair := range transport.Zip(op.left.Next(in), op.right.Next(in)) {
			sides := pair.Read()
			value := sides.Left

			if sides.Right > value {
				value = sides.Right
			}

			if !yield(op.Carrier(value)) {
				return
			}
		}
	}
}
