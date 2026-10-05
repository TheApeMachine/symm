package logic

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Not inverts each arrival.
*/
type Not struct {
	*core.PrimitiveError
	out bool
}

func NewNot() *Not {
	return &Not{
		PrimitiveError: core.NewPrimitiveError(),
	}
}

func (op *Not) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			val := (*bool)(arriving)
			op.out = !*val

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}
