package correlation

import (
	"github.com/theapemachine/symm/nomagique/core"
)

/*
Energy is the total squared energy of a run of returns.

It is the unnormalized variance a correlation is divided by: on an
asynchronously sampled path there is no sample count to divide by that means
the same thing on both sides, so the estimate is scaled by what each path
actually did rather than by how often it happened to be looked at.

The total is of the run it was shown, not of everything it has ever seen. A
composition that wants energy to accumulate across ticks retains it in front
of this, where accumulation is somebody's one job.
*/
type Energy struct {
	core.PrimitiveError
	current core.Primitive
}

/*
NewEnergy configures the value held before anything has been shown.
*/
func NewEnergy(state core.Primitive) *Energy {
	return &Energy{
		current: state,
	}
}

/*
Next gathers the incoming returns and holds their summed square.
*/
func (energy *Energy) Next(in core.Primitive) core.Primitive {
	gathered := core.Yield(
		core.From([]Interval(nil)), in,
		func(held, arriving []Interval) []Interval {
			return append(held, arriving...)
		},
	)

	var total float64

	for _, interval := range core.To[[]Interval](gathered) {
		total += interval.Value * interval.Value
	}

	energy.current = core.From(total)
	energy.current.Error(gathered.Error())

	return energy.current
}

/*
Read surfaces the energy for the boundary.
*/
func (energy *Energy) Read() any {
	return energy.current.Read()
}
