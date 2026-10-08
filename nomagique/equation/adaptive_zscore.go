package equation

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/statistic"
)

/*
AdaptiveZScore uses log-space moments to score arriving observations against
their prior baseline and dispersion.
*/
type AdaptiveZScore struct {
	err     error
	moments statistic.Moments
	out     statistic.CausalResidualResult
}

func NewAdaptiveZScore() core.Primitive {
	return &AdaptiveZScore{}
}

func (op *AdaptiveZScore) Next(
	in iter.Seq[unsafe.Pointer],
) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			val := *(*float64)(arriving)
			logVal := math.Log(val)
			priorMean := op.moments.Mean
			priorCount := op.moments.Count
			priorM2 := op.moments.M2

			reading := op.moments.Update(logVal)

			baseline := val

			if priorCount > 0 {
				baseline = math.Exp(priorMean)
			}

			res := statistic.CausalResidualResult{
				MomentReading: reading,
				HasPrior:      priorCount > 0,
				Baseline:      baseline,
				Residual:      0,
			}

			if priorCount > 0 {
				res.Residual = logVal - priorMean
			}

			if priorCount > 1 {
				res.PriorVariance = priorM2 / (priorCount - 1)
			}

			res.ScoreScale = math.Abs(res.Residual)

			if res.PriorVariance > 0 {
				disp := math.Sqrt(res.PriorVariance)

				if disp > 2.220446049250313e-16 {
					res.ScoreScale = disp
				}
			}

			if res.ScoreScale > 0 {
				res.ZScore = res.Residual / res.ScoreScale
			}

			op.out = res

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

func (op *AdaptiveZScore) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = err
			break
		}
	}

	return op.err
}
