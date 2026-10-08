package algo

import (
	"errors"
	"fmt"
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/correlation"
	"github.com/theapemachine/symm/nomagique/temporal"
)

/*
HayashiYoshida owns asynchronous covariance of two already-decoded return
paths. Each return contributes once to its energy and to every strictly
overlapping cross-product. Support counts overlaps, not independent samples.
*/
type HayashiYoshida struct {
	err error
	out correlation.LagEstimate
}

/*
NewHayashiYoshida creates a new HayashiYoshida primitive.
*/
func NewHayashiYoshida() core.Primitive {
	return &HayashiYoshida{}
}

/*
Next evaluates each arriving return-path pair at its requested timestamp
offset and yields the estimate with its overlap support.
*/
func (op *HayashiYoshida) Next(
	in iter.Seq[unsafe.Pointer],
) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			query := (*correlation.EstimateInput)(arriving)
			reading, err := op.estimate(query)

			if err != nil {
				op.err = errors.Join(op.err, err)
				return
			}

			op.out = reading

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

/*
Error joins every error it observes.
*/
func (op *HayashiYoshida) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}

/*
estimate owns covariance evaluation for one timestamp offset.
*/
func (op *HayashiYoshida) estimate(
	query *correlation.EstimateInput,
) (correlation.LagEstimate, error) {
	if len(query.Left) > 0 {
		through := query.Left[len(query.Left)-1].To
		from := query.Left[0].From

		if (query.Lag > 0 && through > math.MaxInt64-query.Lag) || (query.Lag < 0 && from < math.MinInt64-query.Lag) {
			return correlation.LagEstimate{}, fmt.Errorf(
				"%w: hayashi-yoshida timestamp offset overflows int64",
				core.ErrDomain,
			)
		}
	}

	covariance, support := op.overlap(query.Left, query.Right, query.Lag)
	scale := math.Sqrt(query.LeftEnergy * query.RightEnergy)
	reading := correlation.LagEstimate{
		Correlation: covariance / scale,
		Covariance:  covariance,
		Support:     support,
		LeftEnergy:  query.LeftEnergy,
		RightEnergy: query.RightEnergy,
	}
	reading.Defined = support > 0 && query.LeftEnergy > 0 && query.RightEnergy > 0
	return reading, nil
}

/*
overlap traverses borrowed, ordered return intervals without shifted copies.
*/
func (op *HayashiYoshida) overlap(
	left, right []temporal.LogReturn, lag int64,
) (covariance, support float64) {
	leftIndex, rightIndex := 0, 0

	for leftIndex < len(left) && rightIndex < len(right) {
		leftReturn, rightReturn := left[leftIndex], right[rightIndex]
		leftFrom := leftReturn.From + lag
		leftTo := leftReturn.To + lag

		if leftFrom < rightReturn.To && rightReturn.From < leftTo {
			covariance += leftReturn.Value * rightReturn.Value
			support++
		}

		if leftTo <= rightReturn.To {
			leftIndex++
			continue
		}

		rightIndex++
	}

	return covariance, support
}
