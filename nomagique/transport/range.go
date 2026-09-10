package transport

import (
	"iter"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Range enumerates [0, count) for each arriving count. Iteration state is
private and never leaks into the payload as a control opcode.
*/
type Range[U core.Numeric] struct {
	core.Base[U, U]
}

func NewRange[U core.Numeric]() *Range[U] {
	return &Range[U]{}
}

func (op *Range[U]) Next(
	in iter.Seq[core.Primitive[U, U]],
) iter.Seq[core.Primitive[U, U]] {
	return func(yield func(core.Primitive[U, U]) bool) {
		for arriving := range in {
			count := arriving.Read()

			for index := U(0); index < count; index++ {
				if !yield(op.Carrier(index)) {
					return
				}
			}
		}
	}
}
