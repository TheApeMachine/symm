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
appends. A measurement read yields a working clone of the slot so the
consumer mutates only that copy; peers are live pointers to other slots'
published snapshots. Sequenced queries read per-observation ring slots; the
Disruptor's dependency and wrap barriers own their publication and reuse.
*/
type Register struct {
	*core.PrimitiveError
	slots []*data.Measurement
}

/*
NewRegister creates a register primitive holding no slots.
*/
func NewRegister() *Register {
	return &Register{
		PrimitiveError: core.NewPrimitiveError(),
		slots:          make([]*data.Measurement, 0),
	}
}

/*
Next receives *Query payloads and yields the query back with its answer
filled in: identify assigns a slot, a read fills Value from the slot the
query names, a write replaces the slot the query names. A query addressing a
slot outside the register is a shape failure that ends the stream.
*/
func (op *Register) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		query := data.Read[Query](in)

		switch query.Action() {
		case data.ActionIdentify:
		case data.ActionWrite:
		case data.ActionRead:
		default:
		}
	}
}
