package calculus

import (
	"iter"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Maximum owns one field operation. Configuration supplies the value a run starts
from. What it hands over is the running maximum after every arrival.
*/
type Maximum[U core.Numeric] struct {
	core.Base[U, U]
}

func NewMaximum[U core.Numeric](current U) *Maximum[U] {
	op := &Maximum[U]{}
	op.Carrier(current)
	return op
}

func (op *Maximum[U]) Next(
	in iter.Seq[core.Primitive[U, U]],
) iter.Seq[core.Primitive[U, U]] {
	return func(yield func(core.Primitive[U, U]) bool) {
		for arriving := range in {
			total := op.Read()
			value := arriving.Read()

			if value > total {
				total = value
			}

			if !yield(op.Carrier(total)) {
				return
			}
		}
	}
}
