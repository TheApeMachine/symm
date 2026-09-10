package adaptive

import (
	"iter"
	"math"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
Clock normalizes |value| by the estimator's inclusive mean and applies its
configured pace. A non-positive mean leaves the pace unscaled.
*/
type Clock struct {
	core.Base[float64, float64]
	moments core.Primitive[float64, equation.MomentReading]
	pace    core.Primitive[float64, float64]
}

func NewClock(
	moments core.Primitive[float64, equation.MomentReading],
	pace core.Primitive[float64, float64],
) *Clock {
	return &Clock{moments: moments, pace: pace}
}

func (op *Clock) Next(
	in iter.Seq[core.Primitive[float64, float64]],
) iter.Seq[core.Primitive[float64, float64]] {
	return func(yield func(core.Primitive[float64, float64]) bool) {
		for reading := range op.moments.Next(in) {
			current := reading.Read()
			pace, err := transport.Evaluate(op.pace, transport.Values(current.Value))

			if err != nil {
				op.Error(err)
				return
			}

			ratio := 1.0

			if current.Mean > 0 {
				ratio = math.Abs(current.Value) / current.Mean
			}

			if !yield(op.Carrier(ratio * pace)) {
				return
			}
		}

		op.Error(op.moments.Error())
	}
}
