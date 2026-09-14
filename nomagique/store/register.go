package store

import (
	"iter"
	"strings"
	"sync"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
Register is a fixed-slot O(1) lookup table. Slots are assigned once, when a
subject identifies itself: it is appended and answered its index. From then
on reads and writes are direct slot access — a write replaces, never
appends. All slots are synchronized via RWMutex, and reads of measurement
slots yield isolated clones so concurrent consumers never race on map state.
*/
type Register[T any] struct {
	*core.PrimitiveError
	mu    sync.RWMutex
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
			op.mu.Lock()
			op.slots = append(op.slots, query.payload...)
			slotID := len(op.slots) - 1
			query.Identify(slotID)

			if slotID >= 0 {
				if meas, ok := any(op.slots[slotID]).(*data.Measurement[float64]); ok && meas != nil {
					meas.ID = slotID
				}
			}

			slotVal := op.slots[query.Identity()]
			op.mu.Unlock()

			if !yield(unsafe.Pointer(&slotVal)) {
				return
			}
		case data.ActionWrite:
			op.mu.Lock()

			if query.Identity() < 0 || query.Identity() >= len(op.slots) {
				op.mu.Unlock()
				op.Error(core.ErrShape)
				return
			}

			if len(query.payload) > 0 {
				op.slots[query.Identity()] = query.payload[0]
			}

			slotVal := op.slots[query.Identity()]
			op.mu.Unlock()

			if !yield(unsafe.Pointer(&slotVal)) {
				return
			}
		case data.ActionRead:
			op.mu.RLock()

			if query.Identity() < 0 {
				slotsCopy := make([]T, len(op.slots))
				copy(slotsCopy, op.slots)
				op.mu.RUnlock()

				for index := range slotsCopy {
					if !yield(unsafe.Pointer(&slotsCopy[index])) {
						return
					}
				}

				return
			}

			if query.Identity() >= len(op.slots) {
				op.mu.RUnlock()
				op.Error(core.ErrShape)
				return
			}

			limit := query.Identity()

			if query.PeerLimit() >= 0 {
				limit = query.PeerLimit()
			}

			meas, ok := any(op.slots[query.Identity()]).(*data.Measurement[float64])

			if ok && meas != nil {
				out := any(meas.Clone()).(T)
				outMeas := any(out).(*data.Measurement[float64])
				op.populatePeers(outMeas, query.Identity(), limit)
				op.mu.RUnlock()

				if !yield(unsafe.Pointer(&out)) {
					return
				}

				return
			}

			slotVal := op.slots[query.Identity()]
			op.mu.RUnlock()

			if !yield(unsafe.Pointer(&slotVal)) {
				return
			}
		default:
			op.Error(core.ErrShape)
			return
		}
	}
}

func (op *Register[T]) populatePeers(meas *data.Measurement[float64], slotIndex int, limit int) {
	if meas == nil || meas.Metadata == nil {
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

	if limit > len(op.slots) {
		limit = len(op.slots)
	}

	for idx := 0; idx < limit; idx++ {
		if idx == slotIndex {
			continue
		}

		peer, isMeas := any(op.slots[idx]).(*data.Measurement[float64])

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
