package correlation

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
History maintains the bivariate standardized recurrence trajectory
Z_t = [Z_rho, Z_E]^T and computes nearest-neighbor historical distance
and empirical percentile within the retained pair trajectory history.
Inputs arrive one by one: zScoreRho, zScoreE.
Yields values one by one: distance, percentile.
*/
type History struct {
	*core.PrimitiveError
	points    [][2]float64
	distances []float64
}

func NewHistory() core.Primitive {
	return &History{
		PrimitiveError: core.NewPrimitiveError(),
	}
}

func (op *History) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		var values [2]float64
		idx := 0

		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			if idx < 2 {
				values[idx] = *(*float64)(arriving)
				idx++
			}
		}

		if idx < 2 {
			op.Error(core.ErrShape)
			return
		}

		target := values

		if len(op.points) == 0 {
			op.points = append(op.points, target)

			for value := range data.NewValue(0.0).Next(nil) {
				if !yield(value) {
					return
				}
			}

			for value := range data.NewValue(0.0).Next(nil) {
				if !yield(value) {
					return
				}
			}

			return
		}

		diffRho := target[0] - op.points[0][0]
		diffEnergy := target[1] - op.points[0][1]
		minDistance := math.Sqrt(diffRho*diffRho + diffEnergy*diffEnergy)

		for index := 1; index < len(op.points); index++ {
			diffRho = target[0] - op.points[index][0]
			diffEnergy = target[1] - op.points[index][1]
			distance := math.Sqrt(diffRho*diffRho + diffEnergy*diffEnergy)

			if distance < minDistance {
				minDistance = distance
			}
		}

		percentile := 0.0

		if len(op.distances) > 0 {
			countBelow := 0

			for _, pastDistance := range op.distances {
				if pastDistance <= minDistance {
					countBelow++
				}
			}

			percentile = float64(countBelow) / float64(len(op.distances))
		}

		op.distances = append(op.distances, minDistance)
		op.points = append(op.points, target)

		for value := range data.NewValue(minDistance).Next(nil) {
			if !yield(value) {
				return
			}
		}

		for value := range data.NewValue(percentile).Next(nil) {
			if !yield(value) {
				return
			}
		}
	}
}
