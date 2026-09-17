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
	*core.PrimitiveError

	moments statistic.Moments
	out     statistic.CausalResidualResult
}

func NewAdaptiveZScore() *AdaptiveZScore {
	return &AdaptiveZScore{PrimitiveError: core.NewPrimitiveError()}
}

func (adaptiveZScore *AdaptiveZScore) Next(
	in iter.Seq[unsafe.Pointer],
) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			val := *(*float64)(arriving)
			logVal := math.Log(val)
			priorMean := adaptiveZScore.moments.Mean
			priorCount := adaptiveZScore.moments.Count
			priorM2 := adaptiveZScore.moments.M2

			reading := adaptiveZScore.moments.Update(logVal)

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

				if disp > core.Epsilon {
					res.ScoreScale = disp
				}
			}

			if res.ScoreScale > 0 {
				res.ZScore = res.Residual / res.ScoreScale
			}

			adaptiveZScore.out = res

			if !yield(unsafe.Pointer(&adaptiveZScore.out)) {
				return
			}
		}
	}
}
