package learning

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
DirectionalTarget composes a finite nonnegative deadband and the sign of a delta.
Deadband is configuration of this target.
*/
type DirectionalTarget struct {
	*core.PrimitiveError

	Deadband float64
	out      float64
}

func NewDirectionalTarget(deadband float64) *DirectionalTarget {
	return &DirectionalTarget{PrimitiveError: core.NewPrimitiveError(), Deadband: deadband}
}

func (directionalTarget *DirectionalTarget) Next(
	in iter.Seq[unsafe.Pointer],
) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			sample := (*Observation)(arriving)

			if math.IsNaN(sample.Current) || math.IsNaN(sample.Past) || math.IsNaN(directionalTarget.Deadband) ||
				math.IsInf(sample.Current, 0) || math.IsInf(sample.Past, 0) || math.IsInf(directionalTarget.Deadband, 0) ||
				directionalTarget.Deadband < 0 {
				directionalTarget.Error(core.ErrDomain)
				return
			}

			delta := sample.Current - sample.Past
			magnitude := math.Abs(delta)
			directionalTarget.out = 0.0

			if magnitude > directionalTarget.Deadband {
				directionalTarget.out = math.Copysign(1, delta)
			}

			if !yield(unsafe.Pointer(&directionalTarget.out)) {
				return
			}
		}
	}
}
