package logic

import (
	"iter"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Not owns one Boolean operation. What it hands over is the negation of each
arrival.
*/
type Not struct {
	core.Base[bool, bool]
}

func NewNot() *Not {
	return &Not{}
}

func (op *Not) Next(
	in iter.Seq[core.Primitive[bool, bool]],
) iter.Seq[core.Primitive[bool, bool]] {
	return func(yield func(core.Primitive[bool, bool]) bool) {
		for arriving := range in {
			if !yield(op.Carrier(!arriving.Read())) {
				return
			}
		}
	}
}
