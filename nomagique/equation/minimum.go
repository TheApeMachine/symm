package equation

import (
	"iter"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
Minimum selects the smaller of two configured operations over one input run.
*/
type Minimum[T any, U core.Numeric] struct {
	core.Base[T, U]
	left  core.Primitive[T, U]
	right core.Primitive[T, U]
}

func NewMinimum[T any, U core.Numeric](left, right core.Primitive[T, U]) *Minimum[T, U] {
	return &Minimum[T, U]{left: left, right: right}
}

func (op *Minimum[T, U]) Next(
	in iter.Seq[core.Primitive[T, T]],
) iter.Seq[core.Primitive[U, U]] {
	return func(yield func(core.Primitive[U, U]) bool) {
		for pair := range transport.Zip(op.left.Next(in), op.right.Next(in)) {
			sides := pair.Read()
			value := sides.Left

			if sides.Right < value {
				value = sides.Right
			}

			if !yield(op.Carrier(value)) {
				return
			}
		}
	}
}
