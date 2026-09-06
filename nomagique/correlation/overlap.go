package correlation

import (
	"github.com/theapemachine/symm/nomagique/core"
)

/*
Overlap is the Hayashi-Yoshida pairing: the product of every pair of returns
whose spans overlap in time.

Two paths sampled on their own clocks share no observations to line up, so
the pairing is done in time rather than by index. Returns that coincided
contribute their product; returns that did not, contribute nothing. Nothing
is resampled, interpolated, or bucketed onto a common grid, which is the
whole reason the estimator exists.

The products are handed over as a run rather than reduced, because summing
them is addition's job and counting them is counting's job. Which of those a
composition wants is not for this to decide.

The counter run and the offset are configured Primitives, so one Overlap
serves a contemporaneous estimate and a shifted one without learning about
either. The offset moves the run it is shown, which is how a search asks
whether one path led the other.
*/
type Overlap struct {
	core.PrimitiveError
	counter core.Primitive
	shift   core.Primitive
	current core.Primitive
}

/*
NewOverlap configures the run the incoming returns are paired against, and
the offset in nanoseconds applied to them before pairing.
*/
func NewOverlap(counter, shift core.Primitive) *Overlap {
	return &Overlap{
		counter: counter,
		shift:   shift,
	}
}

/*
Next gathers the incoming returns and holds the products of every pair that
overlapped the counter run.
*/
func (overlap *Overlap) Next(in core.Primitive) core.Primitive {
	gathered := core.Yield(
		core.From([]Interval(nil)), in,
		func(held, arriving []Interval) []Interval {
			return append(held, arriving...)
		},
	)

	counter := core.Yield(
		core.From([]Interval(nil)), overlap.counter,
		func(held, arriving []Interval) []Interval {
			return append(held, arriving...)
		},
	)

	shift := core.Yield(
		core.From(0.0), overlap.shift,
		func(held, value float64) float64 {
			return value
		},
	)

	overlap.current = core.From(products(
		core.To[[]Interval](gathered),
		core.To[[]Interval](counter),
		int64(core.To[float64](shift)),
	))

	overlap.current.Error(gathered.Error(), counter.Error(), shift.Error())

	return overlap.current
}

/*
Read surfaces the products for the boundary.
*/
func (overlap *Overlap) Read() any {
	return overlap.current.Read()
}

/*
products multiplies every overlapping pair of returns, with the shifted run
carrying the offset.

Both runs ascend in time, so the counter scan resumes from the first return
that could still overlap rather than restarting: what has already fallen
behind the shifted span can never come back into it.
*/
func products(shifted, counter []Interval, shift int64) []float64 {
	paired := make([]float64, 0, len(shifted))
	start := 0

	for _, interval := range shifted {
		from, to := interval.From+shift, interval.To+shift

		for start < len(counter) && from >= counter[start].To {
			start++
		}

		for index := start; index < len(counter) && counter[index].From < to; index++ {
			paired = append(paired, interval.Value*counter[index].Value)
		}
	}

	return paired
}
