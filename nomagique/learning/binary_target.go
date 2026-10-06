package learning

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
BinaryTarget classifies an increase without inventing a new numeric rule.
Each arrival is *[2]float64{current, past}; it yields *float64.
*/
type BinaryTarget struct {
	*core.PrimitiveError
	out float64
}

func NewBinaryTarget() core.Primitive {
	return &BinaryTarget{PrimitiveError: core.NewPrimitiveError()}
}

func (op *BinaryTarget) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			sample := (*[2]float64)(arriving)

			if math.IsNaN(sample[0]) || math.IsNaN(sample[1]) ||
				math.IsInf(sample[0], 0) || math.IsInf(sample[1], 0) {
				op.Error(core.ErrDomain)
				return
			}

			op.out = 0.0

			if sample[0] > sample[1] {
				op.out = 1.0
			}

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}
