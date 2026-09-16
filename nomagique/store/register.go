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
appends. A measurement read yields a working clone of the slot so the
consumer mutates only that copy; peers are live pointers to other slots'
published snapshots. Sequenced queries read per-observation ring slots; the
Disruptor's dependency and wrap barriers own their publication and reuse.
*/
type Register[T any] struct {
	*core.PrimitiveError
	slots    []T
	frames   [][]T
	capacity int
}

/*
NewRegister creates a register primitive holding no slots.
*/
func NewRegister[T any](capacity ...int) *Register[T] {
	size := 1

	if len(capacity) > 0 {
		size = capacity[0]
	}

	return &Register[T]{
		capacity:       size,
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
			for ptr := range query.payload {
				op.slots = append(op.slots, *(*T)(ptr))
				op.frames = append(op.frames, make([]T, op.capacity))
			}

			slotID := len(op.slots) - 1
			query.Identify(slotID)

			if slotID >= 0 {
				if meas, ok := any(op.slots[slotID]).(*data.Measurement[float64]); ok && meas != nil {
					meas.ID = slotID
				}
			}

			slotVal := op.slots[query.Identity()]

			if !yield(unsafe.Pointer(&slotVal)) {
				return
			}
		case data.ActionWrite:
			if query.Identity() < 0 || query.Identity() >= len(op.slots) {
				op.Error(core.ErrShape)
				return
			}

			var value T

			if query.payload != nil {
				value = data.Read[T](query.payload)
				op.slots[query.Identity()] = value
			}

			if query.sequence >= 0 {
				op.frames[query.Identity()][query.sequence%int64(op.capacity)] = value
			}

			if !yield(unsafe.Pointer(&value)) {
				return
			}

		case data.ActionRead:
			if query.Identity() < 0 {
				for index := range op.slots {
					value := op.published(index, query.sequence)

					if !yield(unsafe.Pointer(&value)) {
						return
					}
				}

				return
			}

			if query.Identity() >= len(op.slots) {
				op.Error(core.ErrShape)
				return
			}

			slotVal := op.slots[query.Identity()]

			if meas, ok := any(slotVal).(*data.Measurement[float64]); ok && meas != nil {
				working := meas.Clone()
				interest := ""

				if working.Metadata != nil {
					interest = working.Metadata["peer-interest"]
				}

				if interest != "" {
					working.Peers = working.Peers[:0]
					interests := strings.Split(interest, ",")

					for idx := range interests {
						interests[idx] = strings.TrimSpace(interests[idx])
					}

					limit := len(op.slots)

					if query.PeerLimit() >= 0 && query.PeerLimit() < limit {
						limit = query.PeerLimit()
					}

					for idx := 0; idx < limit; idx++ {
						if idx == query.Identity() {
							continue
						}

						value := op.published(idx, query.sequence)

						peer, peerOk := any(value).(*data.Measurement[float64])

						if !peerOk || peer == nil {
							continue
						}

						if matchPeer(peer, interests) {
							working.Peers = append(working.Peers, peer)
						}
					}
				}

				out := any(working).(T)

				if !yield(unsafe.Pointer(&out)) {
					return
				}

				return
			}

			if !yield(unsafe.Pointer(&slotVal)) {
				return
			}
		default:
			op.Error(core.ErrShape)
			return
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

// published reads a sequence slot whose writer has passed the dependency barrier.
func (op *Register[T]) published(identity int, sequence int64) T {
	if sequence >= 0 {
		return op.frames[identity][sequence%int64(op.capacity)]
	}

	return op.slots[identity]
}
