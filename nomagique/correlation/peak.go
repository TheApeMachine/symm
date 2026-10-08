package correlation

import (
	"iter"
	"math"
	"unsafe"

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
	err error
	out PeakResult
}

func NewPeak() core.Primitive {
	return &Peak{}
}

func (op *Peak) Next(
	in iter.Seq[unsafe.Pointer],
) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		var best PeakResult
		seen := false
		index := 0

		for arriving := range in {
			point := (*Point)(arriving)
			magnitude := math.Abs(point.Y)

			if !seen || magnitude > math.Abs(best.Point.Y) {
				best = PeakResult{Index: index, Point: *point}
				seen = true
			}

			index++
		}

		if !seen {
			return
		}

		op.out = best

		if !yield(unsafe.Pointer(&op.out)) {
			return
		}
	}
}

func (op *Peak) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = err
			break
		}
	}

	return op.err
}
