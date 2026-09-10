package equation

import (
	"iter"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Energy owns the sum of squares of each arrival.
*/
type Energy[U core.Numeric] struct {
	core.Base[U, U]
}

func NewEnergy[U core.Numeric]() *Energy[U] {
	op := &Energy[U]{}
	op.Carrier(0)
	return op
}

func (op *Energy[U]) Next(
	in iter.Seq[core.Primitive[U, U]],
) iter.Seq[core.Primitive[U, U]] {
	return func(yield func(core.Primitive[U, U]) bool) {
		for arriving := range in {
			value := arriving.Read()

			if !yield(op.Carrier(op.Read() + value*value)) {
				return
			}
		}
	}
}
