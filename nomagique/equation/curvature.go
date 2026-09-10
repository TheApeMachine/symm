package equation

import (
	"iter"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
Curvature owns neighbouring profile readings around the first absolute peak.
Edge peaks have no neighbours and report ErrShape rather than old evidence.
*/
type Curvature struct {
	core.Base[Point, float64]
	peak *Peak
}

func NewCurvature() *Curvature {
	return &Curvature{peak: NewPeak()}
}

func (op *Curvature) Next(
	in iter.Seq[core.Primitive[Point, Point]],
) iter.Seq[core.Primitive[float64, float64]] {
	return func(yield func(core.Primitive[float64, float64]) bool) {
		var points []Point

		for arriving := range in {
			points = append(points, arriving.Read())
		}

		center := -1

		for result := range op.peak.Next(transport.Values(points...)) {
			center = result.Read().Index
		}

		op.Error(op.peak.Error())

		if center <= 0 || center >= len(points)-1 {
			op.Error(core.ErrShape)
			return
		}

		left := points[center-1]
		mid := points[center]
		right := points[center+1]
		rise := abs(mid.Y) - 0.5*(abs(left.Y)+abs(right.Y))
		run := 0.5 * (right.X - left.X)

		if !yield(op.Carrier(2 * rise / (run * run))) {
			return
		}
	}
}

func abs(value float64) float64 {
	if value < 0 {
		return -value
	}

	return value
}
