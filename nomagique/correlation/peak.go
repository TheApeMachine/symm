package correlation

import (
	"math"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Peak is the strongest reading in a profile, and where it was taken.

A search yields a profile rather than an answer, because the shape around a
maximum is as much of the evidence as the maximum itself. Peak is the one
operation that reduces a profile to its extreme, and it reports the
coordinate with the value so that what follows knows where to look.

Strength is magnitude: a relationship that runs the other way is as strong as
one that runs with, and reading only the largest positive value would find
the second-best answer whenever two paths move against each other.
*/
type Peak struct {
	core.PrimitiveError
	current core.Primitive
}

/*
NewPeak configures the point held before anything has been shown.
*/
func NewPeak(state core.Primitive) *Peak {
	return &Peak{
		current: state,
	}
}

/*
Next gathers the incoming profile and holds its strongest point.
*/
func (peak *Peak) Next(in core.Primitive) core.Primitive {
	gathered := core.Yield(
		core.From([]Point(nil)), in,
		func(held, arriving []Point) []Point {
			return append(held, arriving...)
		},
	)

	profile := core.To[[]Point](gathered)
	index := peakIndex(profile)

	if index < 0 {
		peak.current.Error(gathered.Error())

		return peak.current
	}

	peak.current = core.From(profile[index])
	peak.current.Error(gathered.Error())

	return peak.current
}

/*
Read surfaces the strongest point for the boundary.
*/
func (peak *Peak) Read() any {
	return peak.current.Read()
}

/*
peakIndex is where a profile is strongest, and -1 where it holds nothing. It
is the one place the extreme of a profile is decided, so that reading the
shape around a peak cannot disagree with reading the peak.

A profile holding an unorderable reading has no extreme, so the NaN takes the
peak and the scan stops there rather than stepping over it and reporting the
strongest of what happened to remain comparable. The comparison is strict, so
the first of equal readings stands: an offset that had to be searched for
does not displace one that was already the answer.
*/
func peakIndex(profile []Point) int {
	index := -1
	strongest := math.Inf(-1)

	for position, point := range profile {
		if math.IsNaN(strongest) {
			return index
		}

		if magnitude := math.Abs(point.Y); math.IsNaN(magnitude) ||
			magnitude > strongest {
			strongest, index = magnitude, position
		}
	}

	return index
}
