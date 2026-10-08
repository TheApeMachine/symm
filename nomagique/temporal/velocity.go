package temporal

import (
	"errors"
	"iter"
	"time"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/arithmetic"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
Velocity computes the finite difference rate dValue / dt.
Composes Delta for value change, Delta for elapsed time, and Divide for the rate.
Operands arrive in order: [value, at], where at is in nanoseconds.
*/
type Velocity struct {
	*core.PrimitiveError
	deltaVal core.Primitive
	deltaAt  core.Primitive
	divide   core.Primitive
}

func NewVelocity() core.Primitive {
	return &Velocity{
		PrimitiveError: core.NewPrimitiveError(),
		deltaVal:       NewDelta(),
		deltaAt:        NewDelta(),
		divide:         arithmetic.NewDivide(),
	}
}

func (op *Velocity) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		var values [2]float64
		idx := 0

		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			if idx < 2 {
				values[idx] = *(*float64)(arriving)
				idx++
			}
		}

		if idx < 2 {
			op.Error(core.ErrShape)
			return
		}

		dVal := data.Read[float64](op.deltaVal.Next(data.NewValue(values[0]).Next(nil)))
		dAtNanos := data.Read[float64](op.deltaAt.Next(data.NewValue(values[1]).Next(nil)))

		if err := errors.Join(op.deltaVal.Error(), op.deltaAt.Error()); err != nil {
			op.Error(err)
			return
		}

		dAtSeconds := dAtNanos / float64(time.Second)

		for out := range op.divide.Next(data.NewValue(dVal, dAtSeconds).Next(nil)) {
			if !yield(out) {
				return
			}
		}

		if err := op.divide.Error(); err != nil {
			op.Error(err)
		}
	}
}
