package algo

import (
	"iter"
	"math"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
HayashiYoshida owns asynchronous covariance of two already-decoded return
paths. Each return contributes once to its energy and to every strictly
overlapping cross-product. Support counts overlaps, not independent samples.
*/
type HayashiYoshida struct {
	core.Base[equation.LagProfileInput, equation.LagEstimate]
	paths     [2]equation.LogReturns
	normalize *equation.Correlation[float64]
}

func NewHayashiYoshida() *HayashiYoshida {
	return &HayashiYoshida{normalize: equation.NewCorrelation[float64]()}
}

func (op *HayashiYoshida) Next(
	in iter.Seq[core.Primitive[equation.LagProfileInput, equation.LagProfileInput]],
) iter.Seq[core.Primitive[equation.LagEstimate, equation.LagEstimate]] {
	return func(yield func(core.Primitive[equation.LagEstimate, equation.LagEstimate]) bool) {
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

			estimate, err := op.Estimate(&op.paths[0], &op.paths[1], 0)

			if err != nil {
				op.Error(err)
				return
			}

			if !yield(op.Carrier(estimate)) {
				return
			}
		}
	}
}

/*
Estimate owns covariance evaluation for both direct and lagged callers. Shifting
all timestamps leaves log differences and their energies unchanged.
*/
func (op *HayashiYoshida) Estimate(
	left, right *equation.LogReturns, lag int64,
) (equation.LagEstimate, error) {
	if (lag > 0 && left.Through > math.MaxInt64-lag) ||
		(lag < 0 && left.From < math.MinInt64-lag) {
		return equation.LagEstimate{}, errnie.Error(errnie.Err(
			errnie.Validation,
			"hayashi-yoshida: timestamp offset overflows int64",
			nil,
		))
	}

	covariance, support := op.Overlap(left.Intervals, right.Intervals, lag)
	correlation, err := transport.Evaluate(op.normalize, transport.Values(equation.CorrelationInput[float64]{
		Covariance:  covariance,
		LeftEnergy:  left.Energy,
		RightEnergy: right.Energy,
	}))

	if err != nil {
		return equation.LagEstimate{}, errnie.Error(err)
	}

	return equation.LagEstimate{
		Correlation: correlation,
		Covariance:  covariance,
		Support:     support,
		LeftEnergy:  left.Energy,
		RightEnergy: right.Energy,
	}, nil
}

/*
Overlap traverses borrowed, ordered return intervals without shifted copies.
*/
func (op *HayashiYoshida) Overlap(
	left, right []equation.LogReturn, lag int64,
) (covariance, support float64) {
	leftIndex, rightIndex := 0, 0

	for leftIndex < len(left) && rightIndex < len(right) {
		leftReturn, rightReturn := left[leftIndex], right[rightIndex]
		leftReturn.From += lag
		leftReturn.To += lag

		if leftReturn.From < rightReturn.To && rightReturn.From < leftReturn.To {
			covariance += leftReturn.Value * rightReturn.Value
			support++
		}

		if leftReturn.To <= rightReturn.To {
			leftIndex++
			continue
		}

		rightIndex++
	}

	return covariance, support
}
