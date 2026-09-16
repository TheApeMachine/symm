package algo

import (
	"fmt"
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	nmcorrelation "github.com/theapemachine/symm/nomagique/statistic/correlation"
	"github.com/theapemachine/symm/nomagique/temporal"
)

/*
HayashiYoshida owns asynchronous covariance of two already-decoded return
paths. Each return contributes once to its energy and to every strictly
overlapping cross-product. Support counts overlaps, not independent samples.
*/
type HayashiYoshida struct {
	*core.PrimitiveError

	out nmcorrelation.LagEstimate
}

/*
NewHayashiYoshida creates a new HayashiYoshida primitive.
*/
func NewHayashiYoshida() *HayashiYoshida {
	return &HayashiYoshida{PrimitiveError: core.NewPrimitiveError()}
}

/*
Next evaluates each arriving return-path pair at its requested timestamp
offset and yields the estimate with its overlap support.
*/
func (hayashiYoshida *HayashiYoshida) Next(
	in iter.Seq[unsafe.Pointer],
) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			query := (*nmcorrelation.EstimateInput)(arriving)
			reading, err := hayashiYoshida.estimate(query)

			if err != nil {
				hayashiYoshida.Error(err)
				return
			}

			hayashiYoshida.out = reading

			if !yield(unsafe.Pointer(&hayashiYoshida.out)) {
				return
			}
		}
	}
}

/*
estimate owns covariance evaluation for one timestamp offset.
*/
func (hayashiYoshida *HayashiYoshida) estimate(
	query *nmcorrelation.EstimateInput,
) (nmcorrelation.LagEstimate, error) {
	if len(query.Left) > 0 {
		through := query.Left[len(query.Left)-1].To
		from := query.Left[0].From

		if (query.Lag > 0 && through > math.MaxInt64-query.Lag) || (query.Lag < 0 && from < math.MinInt64-query.Lag) {
			return nmcorrelation.LagEstimate{}, fmt.Errorf(
				"%w: hayashi-yoshida timestamp offset overflows int64",
				core.ErrDomain,
			)
		}
	}

	covariance, support := hayashiYoshida.overlap(query.Left, query.Right, query.Lag)
	scale := math.Sqrt(query.LeftEnergy * query.RightEnergy)
	reading := nmcorrelation.LagEstimate{
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
func (hayashiYoshida *HayashiYoshida) overlap(
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
