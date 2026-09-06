package correlation

import (
	"math"
	"sort"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Median is the middle of a run.

A path's own sampling resolution and its typical energy are both read off
runs where one stretched interval would drag a mean somewhere the path never
was. The median is what makes those readings a description of the path rather
than of its worst moment.

An empty run has no middle, so the value held stands rather than becoming a
zero that would read as a real measurement of nothing.
*/
type Median struct {
	core.PrimitiveError
	current core.Primitive
}

/*
NewMedian configures the value held before anything has been shown.
*/
func NewMedian(state core.Primitive) *Median {
	return &Median{
		current: state,
	}
}

/*
Next gathers the incoming run and holds its middle value.
*/
func (median *Median) Next(in core.Primitive) core.Primitive {
	gathered := core.Yield(
		core.From([]float64(nil)), in,
		func(held, arriving []float64) []float64 {
			return append(held, arriving...)
		},
	)

	values := core.To[[]float64](gathered)

	if len(values) == 0 {
		median.current.Error(gathered.Error())

		return median.current
	}

	median.current = core.From(middleOf(values))
	median.current.Error(gathered.Error())

	return median.current
}

/*
Read surfaces the middle value for the boundary.
*/
func (median *Median) Read() any {
	return median.current.Read()
}

/*
middleOf orders a copy of the run and takes its centre, averaging the two
middle values of an even run. The copy is taken because ordering a run in
place would reorder whatever the caller still holds.

A run holding a NaN has no order, and therefore no middle. Ordering sorts a
NaN to the front, and it is handed straight back rather than stepped over: a
value that cannot be compared must not be quietly dropped so that the
neighbours it invalidates can be reported as a measurement.
*/
func middleOf(values []float64) float64 {
	ordered := append([]float64(nil), values...)
	sort.Float64s(ordered)

	if math.IsNaN(ordered[0]) {
		return ordered[0]
	}

	middle := len(ordered) / 2

	if len(ordered)%2 == 1 {
		return ordered[middle]
	}

	return (ordered[middle-1] + ordered[middle]) / 2
}
