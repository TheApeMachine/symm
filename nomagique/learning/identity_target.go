package learning

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
IdentityTarget selects the finite current value.
*/
type IdentityTarget struct {
	*core.PrimitiveError

	out float64
}

func NewIdentityTarget() *IdentityTarget {
	return &IdentityTarget{PrimitiveError: core.NewPrimitiveError()}
}

func (identityTarget *IdentityTarget) Next(
	in iter.Seq[unsafe.Pointer],
) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			sample := (*Observation)(arriving)

			if math.IsNaN(sample.Current) || math.IsInf(sample.Current, 0) {
				identityTarget.Error(core.ErrDomain)
				return
			}

			identityTarget.out = sample.Current

			if !yield(unsafe.Pointer(&identityTarget.out)) {
				return
			}
		}
	}
}
