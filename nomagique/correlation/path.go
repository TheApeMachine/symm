package correlation

import (
	"github.com/theapemachine/symm/nomagique/core"
)

/*
Path retains a run of observations in ascending event time.

It is retention, not a window: nothing is dropped, because a fixed clamp
silently truncates the evidence an estimate is entitled to. A composition
that wants a bounded path puts a bound in front of the Path rather than
teaching the Path about one.

Time only moves forward on a path, so an observation that regresses is
refused and a repeated timestamp restates the same instant in place. Every
return derived downstream assumes that ordering, and a path that admitted a
regression would hand it a return that ran backwards.
*/
type Path struct {
	core.PrimitiveError
	current core.Primitive
}

/*
NewPath configures the run retained before anything has been observed, which
is ordinarily an empty one.
*/
func NewPath(state core.Primitive) *Path {
	return &Path{
		current: state,
	}
}

/*
Next folds everything the incoming Primitive yields into the retained run and
hands back the result. Offered nothing, Yield answers with the run untouched,
which is how a stage downstream reads the path as a carrier.
*/
func (path *Path) Next(in core.Primitive) core.Primitive {
	path.current = core.Yield(
		path.current, in, func(held, arriving []Observation) []Observation {
			for _, observation := range arriving {
				held = admit(held, observation)
			}

			return held
		},
	)

	return path.current
}

/*
Read surfaces the retained run for the boundary.
*/
func (path *Path) Read() any {
	return path.current.Read()
}

/*
admit places one observation on the run under the ordering that makes a path
a path. The three branches are the trichotomy of the comparison itself: time
advanced, time repeated, or time regressed, and each has exactly one correct
answer.
*/
func admit(held []Observation, observation Observation) []Observation {
	count := len(held)

	if count == 0 || observation.Nanos > held[count-1].Nanos {
		return append(held, observation)
	}

	if observation.Nanos == held[count-1].Nanos {
		held[count-1] = observation
	}

	return held
}
