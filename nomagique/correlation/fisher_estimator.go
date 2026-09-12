package correlation

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/statistic"
)

/*
FisherView is one correlation mapped through atanh, scored against the
configured causal estimator, then mapped back with tanh. Invalid observations
do not advance the estimator.
*/
type FisherView struct {
	Correlation     float64
	Defined         bool
	Baseline        float64
	Divergence      float64
	PriorCount      float64
	Count           float64
	ZScore          float64
	Variance        float64
	VarianceDefined bool
	HasPrior        bool
}

/*
FisherEstimator transforms admissible scalar correlations through atanh,
tracks online moments, and computes causal residuals.
*/
type FisherEstimator struct {
	err     error
	moments statistic.Moments
	out     FisherView
}

func NewFisherEstimator() core.Primitive {
	return &FisherEstimator{}
}

func (op *FisherEstimator) Next(
	in iter.Seq[unsafe.Pointer],
) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			value := *(*float64)(arriving)
			view := FisherView{Correlation: value}

			if value > -1.0 && value < 1.0 {
				z := math.Atanh(value)
				priorMean := op.moments.Mean
				priorCount := op.moments.Count
				priorM2 := op.moments.M2

				reading := op.moments.Update(z)

				baseline := z

				if priorCount > 0 {
					baseline = priorMean
				}

				res := statistic.CausalResidualResult{
					MomentReading: reading,
					HasPrior:      priorCount > 0,
					Baseline:      baseline,
					Residual:      0,
				}

				if priorCount > 0 {
					res.Residual = z - priorMean
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

				view.Defined = true
				view.Baseline = math.Tanh(res.Baseline)
				view.Divergence = res.Residual
				view.PriorCount = priorCount
				view.Count = reading.Count
				view.ZScore = res.ZScore
				view.Variance = reading.Variance
				view.VarianceDefined = reading.VarianceDefined
				view.HasPrior = res.HasPrior
			}

			op.out = view

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

func (op *FisherEstimator) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = err
			break
		}
	}

	return op.err
}
