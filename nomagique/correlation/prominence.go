package correlation

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Prominence owns neighbouring profile readings around the first absolute peak.
Each point arrival is [2]float64{x, y}; it yields *float64. Edge peaks have no
neighbours and report ErrShape.
*/
type Prominence struct {
	*core.PrimitiveError
	out float64
}

func NewProminence() core.Primitive {
	return &Prominence{
		PrimitiveError: core.NewPrimitiveError(),
	}
}

func (op *Prominence) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		var points [][2]float64
		bestIdx := -1
		maxMag := -1.0

		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			point := *(*[2]float64)(arriving)
			idx := len(points)
			points = append(points, point)
			mag := math.Abs(point[1])

			if mag > maxMag {
				maxMag = mag
				bestIdx = idx
			}
		}

		if bestIdx <= 0 || bestIdx >= len(points)-1 {
			op.Error(core.ErrShape)
			return
		}

		left := points[bestIdx-1]
		mid := points[bestIdx]
		right := points[bestIdx+1]

		op.out = math.Abs(mid[1]) - 0.5*(math.Abs(left[1])+math.Abs(right[1]))

		if !yield(unsafe.Pointer(&op.out)) {
			return
		}
	}
}
