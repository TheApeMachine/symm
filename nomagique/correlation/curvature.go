package correlation

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Curvature owns neighbouring profile readings around the first absolute peak.
Edge peaks have no neighbours and report ErrShape.
*/
type Curvature struct {
	err error
	out float64
}

func NewCurvature() core.Primitive {
	return &Curvature{}
}

func (op *Curvature) Next(
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
			op.err = core.ErrShape
			return
		}

		left := points[bestIdx-1]
		mid := points[bestIdx]
		right := points[bestIdx+1]
		rise := math.Abs(mid.Y) - 0.5*(math.Abs(left.Y)+math.Abs(right.Y))
		run := 0.5 * (right.X - left.X)

		op.out = 2.0 * rise / (run * run)

		if !yield(unsafe.Pointer(&op.out)) {
			return
		}
	}
}

func (op *Curvature) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = err
			break
		}
	}

	return op.err
}
