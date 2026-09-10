package correlation

import (
	"iter"
	"math"
	"time"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation"
)

/*
DependenceReading preserves estimator fields and the path diagnostics around one
pair. Finite-sample correlation remains unclipped; zero-energy estimates are
undefined.
*/
type DependenceReading struct {
	equation.LagEstimate
	LeftReturns     float64
	RightReturns    float64
	LeftEnergyRate  float64
	RightEnergyRate float64
	Defined         bool
	SharedTime      float64
	OverlapDensity  float64
}

/*
Dependence owns the typed path diagnostics surrounding an opaque estimator.
*/
type Dependence struct {
	core.Base[equation.LagProfileInput, DependenceReading]
	paths     [2]equation.LogReturns
	estimator equation.LagEstimator
}

func NewDependence(estimator equation.LagEstimator) *Dependence {
	return &Dependence{estimator: estimator}
}

func (op *Dependence) Next(
	in iter.Seq[core.Primitive[equation.LagProfileInput, equation.LagProfileInput]],
) iter.Seq[core.Primitive[DependenceReading, DependenceReading]] {
	return func(yield func(core.Primitive[DependenceReading, DependenceReading]) bool) {
		for arriving := range in {
			input := arriving.Read()

			if err := op.paths[0].Load(input.Left); err != nil {
				op.Error(err)
				return
			}

			if err := op.paths[1].Load(input.Right); err != nil {
				op.Error(err)
				return
			}

			reading, err := op.Summarize(&op.paths[0], &op.paths[1], 0)

			if err != nil {
				op.Error(err)
				return
			}

			if !yield(op.Carrier(reading)) {
				return
			}
		}
	}
}

/*
Estimate shares the caller's decoded paths with the configured estimator.
*/
func (op *Dependence) Estimate(
	left, right *equation.LogReturns, lag int64,
) (equation.LagEstimate, error) {
	return op.estimator.Estimate(left, right, lag)
}

/*
Summarize preserves estimator fields and derives the existing reporting facts.
*/
func (op *Dependence) Summarize(
	left, right *equation.LogReturns, lag int64,
) (DependenceReading, error) {
	estimate, err := op.estimator.Estimate(left, right, lag)

	if err != nil {
		return DependenceReading{}, err
	}

	shared, density := 0.0, 0.0

	if len(left.Intervals) > 0 && len(right.Intervals) > 0 {
		shared = max(0, float64(min(left.Through+lag, right.Through)-max(left.From+lag, right.From))/float64(time.Second))
	}

	if shared > 0 {
		density = estimate.Support / shared
	}

	leftRate, rightRate := math.NaN(), math.NaN()

	if len(left.Intervals) > 0 {
		leftRate = left.MedianEnergyRate()
	}

	if len(right.Intervals) > 0 {
		rightRate = right.MedianEnergyRate()
	}

	return DependenceReading{
		LagEstimate:     estimate,
		LeftReturns:     float64(len(left.Intervals)),
		RightReturns:    float64(len(right.Intervals)),
		LeftEnergyRate:  leftRate,
		RightEnergyRate: rightRate,
		Defined:         estimate.Support > 0 && estimate.LeftEnergy > 0 && estimate.RightEnergy > 0,
		SharedTime:      shared,
		OverlapDensity:  density,
	}, nil
}
