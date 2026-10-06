package learning

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
DirectionalTarget composes a finite nonnegative deadband and the sign of a
delta. Deadband is live configuration of this target.
Each arrival is *[2]float64{current, past}; it yields *float64.
*/
type DirectionalTarget struct {
	*core.PrimitiveError
	Deadband float64
	out      float64
}

func NewDirectionalTarget(deadband float64) core.Primitive {
	return &DirectionalTarget{
		PrimitiveError: core.NewPrimitiveError(),
		Deadband:       deadband,
	}
}

func (op *DirectionalTarget) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			sample := (*[2]float64)(arriving)

			if math.IsNaN(sample[0]) || math.IsNaN(sample[1]) || math.IsNaN(op.Deadband) ||
				math.IsInf(sample[0], 0) || math.IsInf(sample[1], 0) || math.IsInf(op.Deadband, 0) ||
				op.Deadband < 0 {
				op.Error(core.ErrDomain)
				return
			}

			delta := sample[0] - sample[1]
			op.out = 0.0

			if math.Abs(delta) > op.Deadband {
				op.out = math.Copysign(1, delta)
			}

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}
