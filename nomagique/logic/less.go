package logic

import (
	"cmp"
	"iter"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Less owns one ordering relation. Pairing is external: what arrives is already
two values.
*/
type Less[T cmp.Ordered] struct {
	core.Base[[2]T, bool]
}

func NewLess[T cmp.Ordered]() *Less[T] {
	return &Less[T]{}
}

func (op *Less[T]) Next(
	in iter.Seq[core.Primitive[[2]T, [2]T]],
) iter.Seq[core.Primitive[bool, bool]] {
	return func(yield func(core.Primitive[bool, bool]) bool) {
		for arriving := range in {
			pair := arriving.Read()

			if !yield(op.Carrier(pair[0] < pair[1])) {
				return
			}
		}
	}
}
