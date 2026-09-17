package correlation

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/core"
)

/*
Overlap evaluates asynchronous return overlap, unnormalized covariance, and
overlap support across ordered return intervals for a requested timestamp offset.
*/
type Overlap struct {
	*core.PrimitiveError
}

func NewOverlap() *Overlap {
	return &Overlap{
		PrimitiveError: core.NewPrimitiveError(),
	}
}

func (overlap *Overlap) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			query := (*EstimateInput)(arriving)

			if len(query.Left) > 0 {
				through := query.Left[len(query.Left)-1].To
				from := query.Left[0].From

				if (query.Lag > 0 && through > math.MaxInt64-query.Lag) || (query.Lag < 0 && from < math.MinInt64-query.Lag) {
					overlap.Error(errnie.Error(errnie.Err(
						errnie.Validation,
						"overlap: timestamp offset overflows int64",
						core.ErrDomain,
					)))
					return
				}
			}

			covariance := 0.0
			support := 0.0
			leftIndex := 0
			rightIndex := 0

			for leftIndex < len(query.Left) && rightIndex < len(query.Right) {
				leftReturn := query.Left[leftIndex]
				rightReturn := query.Right[rightIndex]
				leftFrom := leftReturn.From + query.Lag
				leftTo := leftReturn.To + query.Lag

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

			out := LagEstimate{
				Covariance:  covariance,
				Support:     support,
				LeftEnergy:  query.LeftEnergy,
				RightEnergy: query.RightEnergy,
				Defined:     support > 0 && query.LeftEnergy > 0 && query.RightEnergy > 0,
			}

			if !yield(unsafe.Pointer(&out)) {
				return
			}
		}
	}
}
