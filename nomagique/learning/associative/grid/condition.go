package grid

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Condition encodes a quantity's identity and the ternary directions of its level
and change into a 64-bit token.
Input: *[3]float64{quantity, level, change}.
Yields: *uint64 token.
*/
type Condition struct {
	*core.PrimitiveError
}

func NewCondition() *Condition {
	return &Condition{
		PrimitiveError: core.NewPrimitiveError(),
	}
}

func (op *Condition) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			values := (*[3]float64)(arriving)
			quantity := uint64(values[0])
			level := values[1]
			change := values[2]

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

			if !yield(unsafe.Pointer(&out)) {
				return
			}
		}
	}
}
