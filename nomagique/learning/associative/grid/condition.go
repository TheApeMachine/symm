package grid

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
Condition encodes a quantity's identity and the ternary directions of its level
and change into a 64-bit token.
*/
type Condition struct {
	*core.PrimitiveError
	Token float64
}

func NewCondition() *Condition {
	return &Condition{
		PrimitiveError: core.NewPrimitiveError(),
	}
}

func (op *Condition) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		var values [3]float64
		index := 0

		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			if index < 3 {
				values[index] = *(*float64)(arriving)
				index++
			}
		}

		if index < 3 {
			op.Error(core.ErrShape)
			return
		}

		quantityVal := values[0]
		level := values[1]
		change := values[2]

		quantity := uint64(quantityVal)

		if quantity == 0 || quantity >= 1<<48 {
			op.Error(core.ErrDomain)
			return
		}

		state := uint64(0)

		if level > 0 {
			state |= 1
		}

		if level < 0 {
			state |= 2
		}

		if change > 0 {
			state |= 1 << 2
		}

		if change < 0 {
			state |= 2 << 2
		}

		out := uint64(1<<52 | quantity<<4 | state)
		op.Token = float64(out)

		for value := range data.NewValue(op.Token).Next(nil) {
			if !yield(value) {
				return
			}
		}
	}
}
