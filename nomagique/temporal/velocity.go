package temporal

import (
	"iter"
	"time"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
Velocity owns the previous observation and computes the finite difference rate.
The first observation and non-advancing time have zero rate.
Operands arrive in order: [value, at], where at is in nanoseconds.
*/
type Velocity struct {
	*core.PrimitiveError
	seen      bool
	prevValue float64
	prevAt    float64
}

func NewVelocity() core.Primitive {
	return &Velocity{
		PrimitiveError: core.NewPrimitiveError(),
	}
}

func (op *Velocity) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		var values [2]*float64

		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			if values[0] == nil {
				values[0] = (*float64)(arriving)
				continue
			}

			values[1] = (*float64)(arriving)
		}

		if values[0] == nil || values[1] == nil {
			return
		}

		value := *values[0]
		at := *values[1]

		if !op.seen {
			op.seen = true
			op.prevValue = value
			op.prevAt = at

			for out := range data.NewValue(0.0).Next(nil) {
				if !yield(out) {
					return
				}
			}
			return
		}

		rate := 0.0
		diffTime := (at - op.prevAt) / float64(time.Second)
		if diffTime > 0 {
			rate = (value - op.prevValue) / diffTime
		}

		op.prevValue = value
		op.prevAt = at

		for out := range data.NewValue(rate).Next(nil) {
			if !yield(out) {
				return
			}
		}
	}
}
