package transport

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Address is a connection with an addressable identity.
*/
type Address[T comparable] struct {
	*core.PrimitiveError
	peer T
	conn core.Primitive
}

/*
NewAddress ...
*/
func NewAddress[T comparable]() *Address[T] {
	return &Address[T]{
		PrimitiveError: core.NewPrimitiveError(),
	}
}

/*
Next ...
*/
func (address *Address[T]) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		if address.conn == nil {
			return
		}

		for i := range address.conn.Next(in) {
			if !yield(i) {
				return
			}
		}
	}
}

func (address *Address[T]) Connect(primitive core.Primitive) {
	address.conn = primitive
}

func (address *Address[T]) Identity() T {
	return address.peer
}

func (address *Address[T]) Identify(identity T) core.Identifiable[T] {
	address.peer = identity
	return address
}
