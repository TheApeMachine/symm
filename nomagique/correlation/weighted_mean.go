package correlation

import (
	"github.com/theapemachine/symm/nomagique/core"
)

/*
WeightedMean is the mean of a run of points where each value carries the
weight it deserves.

A cross-section of correlations is not a set of equal readings: one peer
genuinely co-sampled with the path for an hour, another overlapped it twice.
Averaging them flat lets the second speak as loudly as the first, so each
point pairs its value with the support behind it and the mean is taken over
that.

A run carrying no weight at all leaves the value held standing, since there
is no mean of nothing to report.
*/
type WeightedMean struct {
	core.PrimitiveError
	current core.Primitive
}

/*
NewWeightedMean configures the value held before anything has been shown.
*/
func NewWeightedMean(state core.Primitive) *WeightedMean {
	return &WeightedMean{
		current: state,
	}
}

/*
Next gathers the incoming points, reading X as the weight and Y as the value,
and holds their weighted mean.
*/
func (mean *WeightedMean) Next(in core.Primitive) core.Primitive {
	gathered := core.Yield(
		core.From([]Point(nil)), in,
		func(held, arriving []Point) []Point {
			return append(held, arriving...)
		},
	)

	var weight, weighted float64

	for _, point := range core.To[[]Point](gathered) {
		weight += point.X
		weighted += point.X * point.Y
	}

	if weight == 0 {
		mean.current.Error(gathered.Error())

		return mean.current
	}

	mean.current = core.From(weighted / weight)
	mean.current.Error(gathered.Error())

	return mean.current
}

/*
Read surfaces the weighted mean for the boundary.
*/
func (mean *WeightedMean) Read() any {
	return mean.current.Read()
}
