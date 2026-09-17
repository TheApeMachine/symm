package transport

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
IO is a simple uni-directional pipe for transporting data. It does nothing
more than connect two Primitives together across an arbitrary boundary.
Any additional behavior should be suplied by the two Primitives that are
connected. This does not need to be used by default when two Primitive's
are already connected by means of proximity in a nomagique.Number pipeline.
This is used to connect Primitives over an otherwise unreachable distance.
*/
type IO[T any] struct {
	*core.PrimitiveError
	i core.Primitive
	o core.Primitive
}

/*
NewIO takes the input Primitive and the output Primitive, and keeps them
as the local state. It returns the new IO Primitive instance.
*/
func NewIO[T any](i, o core.Primitive) *IO[T] {
	return &IO[T]{
		PrimitiveError: core.NewPrimitiveError(),
		i:              i,
		o:              o,
	}
}

/*
Next takes the input and proxies it through the input Primitive, then takes
the result of the input Primitive's Next method, and passes that as the input
to the output Primitive.
*/
func (io *IO[T]) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		if io.i != nil {
			defer func() { io.Error(io.i.Error()) }()
		}

		if io.o != nil {
			defer func() { io.Error(io.o.Error()) }()
		}

		stream := in

		if io.i != nil {
			stream = io.i.Next(in)
		}

		if io.o != nil {
			stream = io.o.Next(stream)
		}

		for i := range stream {
			if !yield(i) {
				return
			}
		}
	}
}
