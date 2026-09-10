package equation

import (
	"cmp"
	"iter"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
Less reports whether the left operation is below the right over one input run.
*/
type Less[T any, U cmp.Ordered] struct {
	core.Base[T, bool]
	left  core.Primitive[T, U]
	right core.Primitive[T, U]
}

func NewLess[T any, U cmp.Ordered](left, right core.Primitive[T, U]) *Less[T, U] {
	return &Less[T, U]{left: left, right: right}
}

func (op *Less[T, U]) Next(
	in iter.Seq[core.Primitive[T, T]],
) iter.Seq[core.Primitive[bool, bool]] {
	return func(yield func(core.Primitive[bool, bool]) bool) {
		for pair := range transport.Zip(op.left.Next(in), op.right.Next(in)) {
			sides := pair.Read()

			if !yield(op.Carrier(sides.Left < sides.Right)) {
				return
			}
		}
	}
}
