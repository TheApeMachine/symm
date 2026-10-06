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
paths. Each arrival is *[3][]float64{
  leftFlat returns as {value, from, to, ...},
  rightFlat returns as {value, from, to, ...},
  {leftEnergy, rightEnergy, lag},
}; it yields [6]float64{correlation, covariance, support, leftEnergy,
rightEnergy, defined}. Support counts overlaps, not independent samples.
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
					support++
				}

				if leftTo <= rightTo {
					leftIndex++
					continue
				}

				rightIndex++
			}

			scale := math.Sqrt(leftEnergy * rightEnergy)
			defined := 0.0

			if support > 0 && leftEnergy > 0 && rightEnergy > 0 {
				defined = 1
			}

			op.out = [6]float64{
				covariance / scale,
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
