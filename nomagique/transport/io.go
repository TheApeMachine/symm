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
	i core.Primitive
	o core.Primitive
}

func NewIO(i, o core.Primitive) *IO {
	return &IO{
		i: i,
		o: o,
	}
}

func (io *IO) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return io.o.Next(io.i.Next(in))
}
