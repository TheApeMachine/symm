package algo

import (
	"fmt"
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
HayashiYoshida owns asynchronous covariance of two already-decoded return
paths. Each arrival is *[3][]float64:

	[0] left flat returns as {value, from, to, ...}
	[1] right flat returns as {value, from, to, ...}
	[2] {leftEnergy, rightEnergy, lag}

It yields *[6]float64{correlation, covariance, support, leftEnergy,\nrightEnergy, defined}. Support counts overlaps, not independent samples.\n\nCorrelation normalizes the Hayashi-Yoshida overlap sum with the energies of\nthe same expanded overlap-pair vectors. If one coarse return overlaps several\nfine returns, that coarse return contributes once per overlap to both the\ncovariance numerator and its normalization. This preserves the asynchronous\noverlap estimator while making the published correlation a genuine bounded\ncosine in [-1, 1]. leftEnergy/rightEnergy remain the original path energies\nfor downstream volatility diagnostics.
*/
type HayashiYoshida struct {
	*core.PrimitiveError
	out [6]float64
}

func NewHayashiYoshida() core.Primitive {
	return &HayashiYoshida{
		PrimitiveError: core.NewPrimitiveError(),
	}
}

func (op *HayashiYoshida) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			query := (*[3][]float64)(arriving)

			if len(query[2]) < 3 {
				op.Error(core.ErrShape)
				return
			}

			left := query[0]
			right := query[1]
			leftEnergy := query[2][0]
			rightEnergy := query[2][1]
			lag := query[2][2]

			if len(left) >= 3 {
				through := left[len(left)-1]
				from := left[1]

				if (lag > 0 && through > float64(math.MaxInt64)-lag) || (lag < 0 && from < float64(math.MinInt64)-lag) {
					op.Error(fmt.Errorf(
						"%w: hayashi-yoshida timestamp offset overflows int64",
						core.ErrDomain,
					))
					return
				}
			}

			covariance, support := 0.0, 0.0
			overlapLeftEnergy, overlapRightEnergy := 0.0, 0.0
			leftIndex, rightIndex := 0, 0
			leftCount := len(left) / 3
			rightCount := len(right) / 3

			for leftIndex < leftCount && rightIndex < rightCount {
				leftValue := left[leftIndex*3]
				leftFrom := left[leftIndex*3+1] + lag
				leftTo := left[leftIndex*3+2] + lag
				rightValue := right[rightIndex*3]
				rightFrom := right[rightIndex*3+1]
				rightTo := right[rightIndex*3+2]

				if leftFrom < rightTo && rightFrom < leftTo {
					covariance += leftValue * rightValue
					overlapLeftEnergy += leftValue * leftValue
					overlapRightEnergy += rightValue * rightValue
					support++
				}

				if leftTo <= rightTo {
					leftIndex++
					continue
				}

				rightIndex++
			}

			scale := math.Sqrt(overlapLeftEnergy * overlapRightEnergy)
			defined := 0.0
			correlation := math.NaN()

			if support > 0 && overlapLeftEnergy > 0 && overlapRightEnergy > 0 {
				defined = 1
				correlation = covariance / scale
			}

			op.out = [6]float64{
				correlation,
				covariance,
				support,
				leftEnergy,
				rightEnergy,
				defined,
			}

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}
