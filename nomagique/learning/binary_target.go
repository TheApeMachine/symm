package learning

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
BinaryTarget classifies an increase without inventing a new numeric rule.
*/
type BinaryTarget struct {
	*core.PrimitiveError

	out float64
}

func NewBinaryTarget() *BinaryTarget {
	return &BinaryTarget{PrimitiveError: core.NewPrimitiveError()}
}

func (binaryTarget *BinaryTarget) Next(
	in iter.Seq[unsafe.Pointer],
) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			sample := (*Observation)(arriving)

			if math.IsNaN(sample.Current) || math.IsNaN(sample.Past) ||
				math.IsInf(sample.Current, 0) || math.IsInf(sample.Past, 0) {
				binaryTarget.Error(core.ErrDomain)
				return
			}

			binaryTarget.out = 0.0

			if sample.Current > sample.Past {
				binaryTarget.out = 1.0
			}

			if !yield(unsafe.Pointer(&binaryTarget.out)) {
				return
			}
		}
	}
}
