package logic

import (
	"cmp"
	"iter"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Greater owns one ordering relation. Pairing is external: what arrives is already
two values.
*/
type Greater[T cmp.Ordered] struct {
	core.Base[[2]T, bool]
}

func NewGreater[T cmp.Ordered]() *Greater[T] {
	return &Greater[T]{}
}

func (op *Greater[T]) Next(
	in iter.Seq[core.Primitive[[2]T, [2]T]],
) iter.Seq[core.Primitive[bool, bool]] {
	return func(yield func(core.Primitive[bool, bool]) bool) {
		for arriving := range in {
			pair := arriving.Read()

			if !yield(op.Carrier(pair[0] > pair[1])) {
				return
			}
		}
	}
}
