package store

import (
	"fmt"
	"iter"
	"slices"
	"strconv"
	"strings"
	"unsafe"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
Register is a fixed-slot O(1) lookup table. Slots are assigned once, when a
subject identifies itself: it is appended and answered its index. From then
on reads and writes are direct slot access — a write replaces, never
appends. A measurement read yields a working clone of the slot so the
consumer mutates only that copy; peers are live pointers to other slots'
published snapshots.
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
			for ptr := range query.payload {
				op.slots = append(op.slots, *(*T)(ptr))
			}

			query.Identify(len(op.slots) - 1)

			var (
				measurement *data.Measurement[float64]
				ok          bool
			)

			if measurement, ok = any(
				op.slots[query.Identity()],
			).(*data.Measurement[float64]); ok && measurement != nil {
				measurement.Identify(query.Identity())
			}

			if !yield(unsafe.Pointer(&measurement)) {
				return
			}
		case data.ActionWrite:
			if query.Identity() < 0 || query.Identity() >= len(op.slots) {
				op.Error(core.ErrShape)
				return
			}

			if query.payload != nil {
				op.slots[query.Identity()] = data.Read[T](query.payload)
			}

			if !yield(unsafe.Pointer(&op.slots[query.Identity()])) {
				return
			}
		case data.ActionRead:
			if query.Identity() >= len(op.slots) {
				op.Error(core.ErrShape)
				return
			}

			slotVal := op.slots[query.Identity()]

			if measurement, ok := any(slotVal).(*data.Measurement[float64]); ok && measurement != nil {
				interest := measurement.Metadata["peer-interest"]

				if interest != "" {
					measurement.Peers = measurement.Peers[:0]

					// TODO: Move this into a AddPeer method on Measurement
					//       to lift this out of the hot path.
					interests := strings.Split(strings.ReplaceAll(
						strings.ReplaceAll(interest, " ", ""), fmt.Sprintf(",%d,", query.Identity()), ",",
					), ",")

					if slices.Contains(interests, "*") {
						measurement.Peers = any(
							slices.Clone(op.slots),
						).([]*data.Measurement[float64])

						// We have it all, bail!
						if !yield(unsafe.Pointer(&slotVal)) {
							return
						}
					}

					for _, interest := range interests {
						id, err := strconv.Atoi(interest)

						if err != nil {
							errnie.Error(errnie.Err(
								errnie.Validation,
								"[register] unable to convery peer interest to id",
								err,
							))

							continue
						}

						measurement.Peers = append(
							measurement.Peers,
							any(op.slots[id]).(*data.Measurement[float64]),
						)
					}
				}

				if !yield(unsafe.Pointer(&measurement)) {
					return
				}
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
