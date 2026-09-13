package store

import (
	"iter"
	"strings"
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
			slotID := len(op.slots) - 1
			query.Identify(slotID)

			if slotID >= 0 {
				if meas, ok := any(op.slots[slotID]).(*data.Measurement[float64]); ok && meas != nil {
					meas.ID = slotID
				}
			}

			if !yield(unsafe.Pointer(&op.slots[query.Identity()])) {
				return
			}
		case data.ActionWrite:
			if query.Identity() < 0 || query.Identity() >= len(op.slots) {
				op.Error(core.ErrShape)
				return
			}

			op.slots[query.Identity()] = query.payload[0]

			if !yield(unsafe.Pointer(&op.slots[query.Identity()])) {
				return
			}
		case data.ActionRead:
			if query.Identity() < 0 {
				for index := range op.slots {
					if !yield(unsafe.Pointer(&op.slots[index])) {
						return
					}
				}
				return
			}

			if query.Identity() >= len(op.slots) {
				op.Error(core.ErrShape)
				return
			}

			op.populatePeers(query.Identity())

			if !yield(unsafe.Pointer(&op.slots[query.Identity()])) {
				return
			}
		default:
			op.Error(core.ErrShape)
			return
		}
	}
}

func (op *Register[T]) populatePeers(slotIndex int) {
	meas, ok := any(op.slots[slotIndex]).(*data.Measurement[float64])

	if !ok || meas == nil || meas.Metadata == nil {
		return
	}

	interest, holds := meas.Metadata["peer-interest"]

	if !holds || interest == "" {
		return
	}

	meas.Peers = meas.Peers[:0]
	interests := strings.Split(interest, ",")

	for idx := range interests {
		interests[idx] = strings.TrimSpace(interests[idx])
	}

	for idx, slot := range op.slots {
		if idx == slotIndex {
			continue
		}

		peer, isMeas := any(slot).(*data.Measurement[float64])

		if !isMeas || peer == nil {
			continue
		}

		if matchPeer(peer, interests) {
			meas.Peers = append(meas.Peers, peer.Clone())
		}
	}
}

func matchPeer(peer *data.Measurement[float64], interests []string) bool {
	for _, interest := range interests {
		if interest == "*" || interest == peer.Source || interest == peer.Label {
			return true
		}
	}

	return false
}
