package learning

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
IdentityTarget selects the finite current value.
Each arrival is *[2]float64{current, past}; it yields *float64.
*/
type IdentityTarget struct {
	*core.PrimitiveError
	out float64
}

func NewIdentityTarget() core.Primitive {
	return &IdentityTarget{PrimitiveError: core.NewPrimitiveError()}
}

func (op *IdentityTarget) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			sample := (*[2]float64)(arriving)

			if math.IsNaN(sample[0]) || math.IsInf(sample[0], 0) {
				op.Error(core.ErrDomain)
				return
			}

			op.out = sample[0]

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}
