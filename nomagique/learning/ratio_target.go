package learning

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
RatioTarget is the relative change, with an explicit nonzero past domain.
*/
type RatioTarget struct {
	*core.PrimitiveError

	out float64
}

func NewRatioTarget() *RatioTarget {
	return &RatioTarget{PrimitiveError: core.NewPrimitiveError()}
}

func (ratioTarget *RatioTarget) Next(
	in iter.Seq[unsafe.Pointer],
) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			sample := (*Observation)(arriving)

			if math.IsNaN(sample.Current) || math.IsNaN(sample.Past) ||
				math.IsInf(sample.Current, 0) || math.IsInf(sample.Past, 0) ||
				sample.Past == 0 {
				ratioTarget.Error(core.ErrDomain)
				return
			}

			ratioTarget.out = sample.Current/sample.Past - 1

			if !yield(unsafe.Pointer(&ratioTarget.out)) {
				return
			}
		}
	}
}
