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

It reports the covariance with its standard error, never a correlation: the
covariance normalized by the full-path energies is not bounded by one on
asynchronous grids (one return can overlap several), and forcing it into
[-1, 1] would hide that. The standard error is the plug-in null scale,
sqrt(sum over overlapping pairs of r_i^2 s_j^2), which is the standard
deviation of the Hayashi-Yoshida sum when the two paths have independent
increments. Score is covariance / standard error, defined when that scale is
positive.
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

	covariance, support, nullVariance := op.overlap(query.Left, query.Right, query.Lag)

	reading := correlation.LagEstimate{
		Covariance:  covariance,
		Support:     support,
		LeftEnergy:  query.LeftEnergy,
		RightEnergy: query.RightEnergy,
	}
	reading.Defined = support > 0 && query.LeftEnergy > 0 && query.RightEnergy > 0

	if reading.Defined && nullVariance > 0 {
		reading.StandardError = math.Sqrt(nullVariance)
		reading.Score = covariance / reading.StandardError
		reading.ScoreDefined = true
	}

	return reading, nil
}

/*
overlap traverses borrowed, ordered return intervals without shifted copies,
accumulating the cross-products and their squared terms.
*/
func (op *HayashiYoshida) overlap(
	left, right []temporal.LogReturn, lag int64,
) (covariance, support, nullVariance float64) {
	leftIndex, rightIndex := 0, 0

	for leftIndex < len(left) && rightIndex < len(right) {
		leftReturn, rightReturn := left[leftIndex], right[rightIndex]
		leftFrom := leftReturn.From + lag
		leftTo := leftReturn.To + lag

		if leftFrom < rightReturn.To && rightReturn.From < leftTo {
			product := leftReturn.Value * rightReturn.Value
			covariance += product
			nullVariance += product * product
			support++
		}

		if leftTo <= rightReturn.To {
			leftIndex++
			continue
		}

		rightIndex++
	}

	return covariance, support, nullVariance
}
