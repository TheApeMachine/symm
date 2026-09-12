package store

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
Register is a fixed-slot O(1) lookup table. Slots are assigned once, when a
subject identifies itself: it is appended and answered its index. From then
on reads and writes are direct slot access — a write replaces, never
appends. Because every subject writes only the slot it was assigned, no
locking is needed. It stores data; it knows nothing about who queries it or
why. Every store in the system answers the same Query protocol.
*/
type Register[T any] struct {
	*core.PrimitiveError
	slots []T
}

/*
NewRegister creates a register primitive holding no slots.
*/
func NewRegister[T any]() *Register[T] {
	return &Register[T]{
		PrimitiveError: core.NewPrimitiveError(),
	}
}

/*
Next receives *Query payloads and yields the query back with its answer
filled in: identify assigns a slot, a read fills Value from the slot the
query names, a write replaces the slot the query names. A query addressing a
slot outside the register is a shape failure that ends the stream.
*/
func (op *Register[T]) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		query := data.Read[Query[T]](in)

		switch query.Action() {
		case data.ActionIdentify:
			op.slots = append(op.slots, query.payload...)
			query.Identify(len(op.slots) - 1)
		case data.ActionRead, data.ActionWrite:
			if query.Identity() < 0 || query.Identity() >= len(op.slots) {
				op.Error(core.ErrShape)
				return
			}

			if query.Action() == data.ActionWrite {
				op.slots[query.Identity()] = query.payload[0]
			}
		default:
			op.Error(core.ErrShape)
			return
		}

		if !yield(unsafe.Pointer(&op.slots[query.Identity()])) {
			return
		}
	}
}
