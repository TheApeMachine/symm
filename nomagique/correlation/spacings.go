package correlation

import (
	"github.com/theapemachine/symm/nomagique/core"
)

/*
Spacings is the run of gaps in nanoseconds between consecutive observations:
how often a path was actually sampled.

It exists so that nothing downstream has to invent a grid. Reduced to its
median, this is a path's own resolution, and a search that strides by it can
neither step over a relationship nor pretend to a precision the path never
had.

A gap that does not advance is not a gap, so it contributes nothing.
*/
type Spacings struct {
	core.PrimitiveError
	current core.Primitive
}

/*
NewSpacings configures the run held before anything has been shown.
*/
func NewSpacings(state core.Primitive) *Spacings {
	return &Spacings{
		current: state,
	}
}

/*
Next gathers the incoming observations and holds the gaps between them.
*/
func (spacings *Spacings) Next(in core.Primitive) core.Primitive {
	gathered := core.Yield(
		core.From([]Observation(nil)), in,
		func(held, arriving []Observation) []Observation {
			return append(held, arriving...)
		},
	)

	spacings.current = core.From(
		gapsOf(core.To[[]Observation](gathered)),
	)

	spacings.current.Error(gathered.Error())

	return spacings.current
}

/*
Read surfaces the gaps for the boundary.
*/
func (spacings *Spacings) Read() any {
	return spacings.current.Read()
}

/*
gapsOf differences consecutive timestamps into the intervals between them.
*/
func gapsOf(observations []Observation) []float64 {
	gaps := make([]float64, 0, len(observations))

	for index := 1; index < len(observations); index++ {
		elapsed := observations[index].Nanos - observations[index-1].Nanos

		if elapsed <= 0 {
			continue
		}

		gaps = append(gaps, float64(elapsed))
	}

	return gaps
}
