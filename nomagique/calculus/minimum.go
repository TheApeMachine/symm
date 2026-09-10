package calculus

import (
	"iter"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Minimum owns one field operation. Configuration supplies the value a run starts
from. What it hands over is the running minimum after every arrival.
*/
type Minimum[U core.Numeric] struct {
	core.Base[U, U]
}

func NewMinimum[U core.Numeric](current U) *Minimum[U] {
	op := &Minimum[U]{}
	op.Carrier(current)
	return op
}

func (op *Minimum[U]) Next(
	in iter.Seq[core.Primitive[U, U]],
) iter.Seq[core.Primitive[U, U]] {
	return func(yield func(core.Primitive[U, U]) bool) {
		for arriving := range in {
			total := op.Read()
			value := arriving.Read()

			if value < total {
				total = value
			}

			if !yield(op.Carrier(total)) {
				return
			}
		}
	}
}
