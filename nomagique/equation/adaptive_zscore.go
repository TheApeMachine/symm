package equation

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/statistic"
)

/*
AdaptiveZScore uses log-space moments to score arriving observations against
their prior baseline and dispersion. It composes an Estimator over the log of
each arriving positive *float64 with a CausalResidual, and yields the CausalResidual
layout as *[8]float64 with the baseline mapped back out of log space:

	[0] has prior (1 or 0) [1] baseline    [2] prior variance [3] maturity
	[4] residual           [5] score scale [6] z-score        [7] noise variance
*/
type AdaptiveZScore struct {
	*core.PrimitiveError
	moments  core.Primitive
	residual core.Primitive
	out      [8]float64
}

func NewAdaptiveZScore() *AdaptiveZScore {
	return &AdaptiveZScore{
		PrimitiveError: core.NewPrimitiveError(),
		moments:        statistic.NewEstimator(),
		residual:       statistic.NewCausalResidual(),
	}
}

func (op *AdaptiveZScore) Next(
	in iter.Seq[unsafe.Pointer],
) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			value := *(*float64)(arriving)

			if value <= 0 {
				op.Error(core.ErrDomain)
				return
			}

			logValue := math.Log(value)

			for reading := range op.moments.Next(data.NewValue(logValue)) {
				for pointer := range op.residual.Next(data.NewValue(*(*[10]float64)(reading))) {
					op.out = *(*[8]float64)(pointer)
				}
			}

			if err := op.moments.Error(); err != nil {
				op.Error(err)
				return
			}

			if err := op.residual.Error(); err != nil {
				op.Error(err)
				return
			}

			op.out[1] = math.Exp(op.out[1])

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}
