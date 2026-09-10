package logic

import (
	"iter"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
And owns one Boolean operation. Configuration supplies the value a run starts
from. What it hands over is the running conjunction after every arrival.
*/
type And struct {
	core.Base[bool, bool]
}

func NewAnd(current bool) *And {
	op := &And{}
	op.Carrier(current)
	return op
}

func (op *And) Next(
	in iter.Seq[core.Primitive[bool, bool]],
) iter.Seq[core.Primitive[bool, bool]] {
	return func(yield func(core.Primitive[bool, bool]) bool) {
		for arriving := range in {
			if !yield(op.Carrier(op.Read() && arriving.Read())) {
				return
			}
		}
	}
}
