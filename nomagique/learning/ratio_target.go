package learning

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
RatioTarget is the relative change, with an explicit nonzero past domain.
Each arrival is *[2]float64{current, past}; it yields *float64.
*/
type RatioTarget struct {
	*core.PrimitiveError
	out float64
}

func NewRatioTarget() core.Primitive {
	return &RatioTarget{PrimitiveError: core.NewPrimitiveError()}
}

func (op *RatioTarget) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			sample := (*[2]float64)(arriving)

			if math.IsNaN(sample[0]) || math.IsNaN(sample[1]) ||
				math.IsInf(sample[0], 0) || math.IsInf(sample[1], 0) ||
				sample[1] == 0 {
				op.Error(core.ErrDomain)
				return
			}

			op.out = sample[0]/sample[1] - 1

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}
