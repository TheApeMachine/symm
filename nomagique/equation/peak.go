package equation

import (
	"iter"
	"math"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Point is an ordinate in a profile. x coordinates determine units.
*/
type Point struct {
	X float64
	Y float64
}

/*
PeakResult is the first absolute maximum and where it occurred.
*/
type PeakResult struct {
	Index int
	Point Point
}

/*
Peak owns one delivery's maximum absolute ordinate and its original point.
*/
type Peak struct {
	core.Base[Point, PeakResult]
}

func NewPeak() *Peak {
	return &Peak{}
}

func (op *Peak) Next(
	in iter.Seq[core.Primitive[Point, Point]],
) iter.Seq[core.Primitive[PeakResult, PeakResult]] {
	return func(yield func(core.Primitive[PeakResult, PeakResult]) bool) {
		var best PeakResult
		seen := false
		index := 0

		for arriving := range in {
			point := arriving.Read()
			magnitude := math.Abs(point.Y)

			if !seen || magnitude > math.Abs(best.Point.Y) {
				best = PeakResult{Index: index, Point: point}
				seen = true
			}

			index++
		}

		if !seen {
			return
		}

		if !yield(op.Carrier(best)) {
			return
		}
	}
}
