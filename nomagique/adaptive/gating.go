package adaptive

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	sequence "github.com/theapemachine/symm/nomagique/data/sequence"
	"github.com/theapemachine/symm/nomagique/statistic"
)

/*
Gating suppresses values inside a configured threshold of inclusive moments.
*/
type Gating struct {
	*core.PrimitiveError

	moments   core.Primitive
	threshold core.Primitive
	out       float64
}

func NewGating(
	moments core.Primitive,
	threshold core.Primitive,
) *Gating {
	return &Gating{PrimitiveError: core.NewPrimitiveError(), moments: moments, threshold: threshold}
}

func (gating *Gating) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		defer func() {

			if gating.moments != nil {
				if err := gating.moments.Error(); err != nil {
					gating.Error(err)
				}
			}

			if gating.threshold != nil {
				if err := gating.threshold.Error(); err != nil {
					gating.Error(err)
				}
			}
		}()
		for readingPtr := range gating.moments.Next(in) {
			current := *(*statistic.MomentReading)(readingPtr)
			limitEval := gating.threshold
			var limit float64

			for out := range limitEval.Next(sequence.NewValues(current.Count).Next(nil)) {
				limit = *(*float64)(out)
			}

			err := limitEval.Error()

			if err != nil {
				gating.Error(err)
				return
			}

			gating.out = current.Value

			if current.Dispersion > 0 && math.Abs(current.Value-current.Mean) < limit {
				gating.out = 0
			}

			if !yield(unsafe.Pointer(&gating.out)) {
				return
			}
		}
	}
}
