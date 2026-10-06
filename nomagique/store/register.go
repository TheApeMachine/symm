package store

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
Register is a fixed-slot O(1) table. Slots are assigned once, when a
measurement arrives: it is appended and answered its slot index. A nil run
plays the slots back in slot order.
*/
type Register struct {
	*core.PrimitiveError
	slots []*data.Measurement
	index int
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

func (op *Register) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		if in == nil {
			for index := range op.slots {
				if !yield(unsafe.Pointer(&op.slots[index])) {
					return
				}
			}

			return
		}

		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			measurement := *(**data.Measurement)(arriving)

			if measurement == nil {
				op.Error(core.ErrShape)
				return
			}

			op.slots = append(op.slots, measurement)
			op.index = len(op.slots) - 1

			if !yield(unsafe.Pointer(&op.index)) {
				return
			}
		}
	}
}
