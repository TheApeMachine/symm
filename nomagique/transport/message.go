package transport

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Message is some data exchanged between two primitives. If address is set, the message
will become addressed, which means it will only be delivered to the addressed peer, if
address is left nil, the Message will blindly pass through. If action is set the message
acts similar to an event.
*/
type Message[T comparable] struct {
	*core.PrimitiveError
	address core.Identifiable[T]
	action  core.Action
}

/*
NewMessage ...
*/
func NewMessage[T comparable](
	address core.Identifiable[T],
	action core.Action,
	data core.Primitive,
) *Message[T] {
	return &Message[T]{
		PrimitiveError: core.NewPrimitiveError(),
		address:        address,
		action:         action,
	}
}

/*
Next ...
*/
func (message *Message[T]) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		if message.address != nil {
			concrete := any(in).(core.Identifiable[T])

			if concrete.Identity() != message.address.Identity() {
				return
			}
		}

		for i := range in {
			if !yield(i) {
				return
			}
		}
	}
}
