package correlation

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Prominence owns neighbouring profile readings around the first absolute peak.
Edge peaks have no neighbours and report ErrShape.
*/
type Prominence struct {
	*core.PrimitiveError

	out float64
}

func NewProminence() *Prominence {
	return &Prominence{PrimitiveError: core.NewPrimitiveError()}
}

func (prominence *Prominence) Next(
	in iter.Seq[unsafe.Pointer],
) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		var points []Point
		bestIdx := -1
		maxMag := -1.0

		for arriving := range in {
			point := *(*Point)(arriving)
			idx := len(points)
			points = append(points, point)
			mag := math.Abs(point.Y)

			if mag > maxMag {
				maxMag = mag
				bestIdx = idx
			}
		}

		if bestIdx <= 0 || bestIdx >= len(points)-1 {
			prominence.Error(core.ErrShape)
			return
		}

		left := points[bestIdx-1]
		mid := points[bestIdx]
		right := points[bestIdx+1]

		prominence.out = math.Abs(mid.Y) - 0.5*(math.Abs(left.Y)+math.Abs(right.Y))

		if !yield(unsafe.Pointer(&prominence.out)) {
			return
		}
	}
}
