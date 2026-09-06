package correlation

import (
	"math"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Returns is the log-return of a run of observations: what the path did between
one observation and the next, expressed so that moves compound by addition.

A return is only defined where time strictly advanced and both endpoints are
positive, so a stalled clock or a non-positive reading contributes no return
rather than a spurious one. That is the difference operator's own domain, not
a filter applied to taste.

Yield gathers the incoming run, because this operation is over a whole run
rather than over each value in isolation, and the transform is applied to
what Yield handed back. Nothing here inspects the neighbour that produced it.
*/
type Returns struct {
	core.PrimitiveError
	current core.Primitive
}

/*
NewReturns configures the run held before anything has been shown.
*/
func NewReturns(state core.Primitive) *Returns {
	return &Returns{
		current: state,
	}
}

/*
Next gathers the incoming observations and holds their returns.
*/
func (returns *Returns) Next(in core.Primitive) core.Primitive {
	gathered := core.Yield(
		core.From([]Observation(nil)), in,
		func(held, arriving []Observation) []Observation {
			return append(held, arriving...)
		},
	)

	returns.current = core.From(
		intervalsOf(core.To[[]Observation](gathered)),
	)

	returns.current.Error(gathered.Error())

	return returns.current
}

/*
Read surfaces the returns for the boundary.
*/
func (returns *Returns) Read() any {
	return returns.current.Read()
}

/*
intervalsOf differences consecutive observations into the returns they
realized, keeping the span each one covers so a later estimate can ask which
returns coincided in time.
*/
func intervalsOf(observations []Observation) []Interval {
	intervals := make([]Interval, 0, len(observations))

	for index := 1; index < len(observations); index++ {
		previous, current := observations[index-1], observations[index]

		if previous.Nanos >= current.Nanos ||
			previous.Value <= 0 || current.Value <= 0 {
			continue
		}

		intervals = append(intervals, Interval{
			From:  previous.Nanos,
			To:    current.Nanos,
			Value: math.Log(current.Value / previous.Value),
		})
	}

	return intervals
}
