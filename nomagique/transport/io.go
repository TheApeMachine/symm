package transport

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
IO is a pipe between two primitives.
*/
type IO struct {
	*core.PrimitiveError
	i core.Primitive
	o core.Primitive
}

func NewIO(i, o core.Primitive) core.Primitive {
	return &IO{
		PrimitiveError: core.NewPrimitiveError(),
		i:              i,
		o:              o,
	}
}

func (op *IO) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for out := range op.o.Next(op.i.Next(in)) {
			if !yield(out) {
				return
			}
		}

		op.Error(op.i.Error(), op.o.Error())
	}
}
