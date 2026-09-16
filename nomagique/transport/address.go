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
	peer core.Identifiable[T]
	conn core.Primitive
}

/*
NewAddress ...
*/
func NewAddress[T comparable](peer core.Identifiable[T]) *Address[T] {
	return &Address[T]{
		PrimitiveError: core.NewPrimitiveError(),
		peer:           peer,
	}
}

/*
Next ...
*/
func (address *Address[T]) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		concrete := any(in).(core.Identifiable[T])

		if concrete.Identity() == address.peer.Identity() {
			for i := range address.conn.Next(in) {
				if !yield(i) {
					return
				}
			}
		}
	}
}
