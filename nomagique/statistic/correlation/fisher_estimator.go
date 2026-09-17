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
	*core.PrimitiveError

	moments statistic.Moments
}

func NewFisherEstimator() *FisherEstimator {
	return &FisherEstimator{PrimitiveError: core.NewPrimitiveError()}
}

func (fisherEstimator *FisherEstimator) Next(
	in iter.Seq[unsafe.Pointer],
) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			value := *(*float64)(arriving)
			view := FisherView{Correlation: value}

			if value > -core.Unit && value < core.Unit {
				z := math.Atanh(value)
				priorMean := fisherEstimator.moments.Mean
				priorCount := fisherEstimator.moments.Count
				priorM2 := fisherEstimator.moments.M2

				reading := fisherEstimator.moments.Update(z)

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

				if priorCount > core.Unit {
					res.PriorVariance = priorM2 / (priorCount - core.Unit)
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

			if !yield(unsafe.Pointer(&view)) {
				return
			}
		}
	}
}

/*
FisherBaseline yields the tanh-mapped causal Fisher-space baseline.
*/
type FisherBaseline struct {
	*core.PrimitiveError

	out float64
}

func NewFisherBaseline() *FisherBaseline {
	return &FisherBaseline{PrimitiveError: core.NewPrimitiveError()}
}

func (fisherBaseline *FisherBaseline) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			view := (*FisherView)(arriving)

			if !view.Defined {
				continue
			}

			fisherBaseline.out = view.Baseline

			if !yield(unsafe.Pointer(&fisherBaseline.out)) {
				return
			}
		}
	}
}

/*
FisherDivergence yields the Fisher-space residual from the causal baseline.
*/
type FisherDivergence struct {
	*core.PrimitiveError

	out float64
}

func NewFisherDivergence() *FisherDivergence {
	return &FisherDivergence{PrimitiveError: core.NewPrimitiveError()}
}

func (fisherDivergence *FisherDivergence) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			view := (*FisherView)(arriving)

			if !view.Defined {
				continue
			}

			fisherDivergence.out = view.Divergence

			if !yield(unsafe.Pointer(&fisherDivergence.out)) {
				return
			}
		}
	}
}

/*
FisherZScore yields the Fisher-space residual in units of prior noise.
*/
type FisherZScore struct {
	*core.PrimitiveError

	out float64
}

func NewFisherZScore() *FisherZScore {
	return &FisherZScore{PrimitiveError: core.NewPrimitiveError()}
}

func (fisherZScore *FisherZScore) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			view := (*FisherView)(arriving)

			if !view.Defined {
				continue
			}

			fisherZScore.out = view.ZScore

			if !yield(unsafe.Pointer(&fisherZScore.out)) {
				return
			}
		}
	}
}
