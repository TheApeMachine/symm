package logic

import (
	"iter"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Or owns one Boolean operation. Configuration supplies the value a run starts
from. What it hands over is the running disjunction after every arrival.
*/
type Or struct {
	core.Base[bool, bool]
}

func NewOr(current bool) *Or {
	op := &Or{}
	op.Carrier(current)
	return op
}

func (op *Or) Next(
	in iter.Seq[core.Primitive[bool, bool]],
) iter.Seq[core.Primitive[bool, bool]] {
	return func(yield func(core.Primitive[bool, bool]) bool) {
		for arriving := range in {
			if !yield(op.Carrier(op.Read() || arriving.Read())) {
				return
			}
		}
	}
}
