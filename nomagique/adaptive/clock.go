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
Clock normalizes |value| by the estimator's inclusive mean and applies its
configured pace. A non-positive mean leaves the pace unscaled.
*/
type Clock struct {
	*core.PrimitiveError

	moments core.Primitive
	pace    core.Primitive
	out     float64
}

func NewClock(
	moments core.Primitive,
	pace core.Primitive,
) *Clock {
	return &Clock{PrimitiveError: core.NewPrimitiveError(), moments: moments, pace: pace}
}

func (clock *Clock) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		defer func() {

			if clock.moments != nil {
				if err := clock.moments.Error(); err != nil {
					clock.Error(err)
				}
			}

			if clock.pace != nil {
				if err := clock.pace.Error(); err != nil {
					clock.Error(err)
				}
			}
		}()
		for readingPtr := range clock.moments.Next(in) {
			current := *(*statistic.MomentReading)(readingPtr)
			paceValEval := clock.pace
			var paceVal float64

			for out := range paceValEval.Next(sequence.NewValues(current.Value).Next(nil)) {
				paceVal = *(*float64)(out)
			}

			err := paceValEval.Error()

			if err != nil {
				clock.Error(err)
				return
			}

			ratio := 1.0

			if current.Mean > 0 {
				ratio = math.Abs(current.Value) / current.Mean
			}

			clock.out = ratio * paceVal

			if !yield(unsafe.Pointer(&clock.out)) {
				return
			}
		}
	}
}
