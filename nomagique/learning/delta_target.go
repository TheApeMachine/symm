package learning

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
DeltaTarget returns the observed current-minus-past difference.
*/
type DeltaTarget struct {
	*core.PrimitiveError

	out float64
}

func NewDeltaTarget() *DeltaTarget {
	return &DeltaTarget{PrimitiveError: core.NewPrimitiveError()}
}

func (deltaTarget *DeltaTarget) Next(
	in iter.Seq[unsafe.Pointer],
) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			sample := (*Observation)(arriving)

			if math.IsNaN(sample.Current) || math.IsNaN(sample.Past) ||
				math.IsInf(sample.Current, 0) || math.IsInf(sample.Past, 0) {
				deltaTarget.Error(core.ErrDomain)
				return
			}

			deltaTarget.out = sample.Current - sample.Past

			if !yield(unsafe.Pointer(&deltaTarget.out)) {
				return
			}
		}
	}
}
