package correlation

import (
	"github.com/theapemachine/symm/nomagique/core"
)

/*
EffectiveCount is how many equally-weighted points a weighted run is worth.

It is Kish's effective sample size, and it is what stops a weighted mean from
reporting a cross-section it does not have: a cohort of forty peers where one
overlapped a thousand times and the rest twice is one peer's opinion wearing
forty names, and this says so by reporting close to one.

A run carrying no weight leaves the value held standing.
*/
type EffectiveCount struct {
	core.PrimitiveError
	current core.Primitive
}

/*
NewEffectiveCount configures the value held before anything has been shown.
*/
func NewEffectiveCount(state core.Primitive) *EffectiveCount {
	return &EffectiveCount{
		current: state,
	}
}

/*
Next gathers the incoming points, reading X as the weight, and holds the
effective count of the run.
*/
func (count *EffectiveCount) Next(in core.Primitive) core.Primitive {
	gathered := core.Yield(
		core.From([]Point(nil)), in,
		func(held, arriving []Point) []Point {
			return append(held, arriving...)
		},
	)

	var weight, squares float64

	for _, point := range core.To[[]Point](gathered) {
		weight += point.X
		squares += point.X * point.X
	}

	if squares == 0 {
		count.current.Error(gathered.Error())

		return count.current
	}

	count.current = core.From(weight * weight / squares)
	count.current.Error(gathered.Error())

	return count.current
}

/*
Read surfaces the effective count for the boundary.
*/
func (count *EffectiveCount) Read() any {
	return count.current.Read()
}
