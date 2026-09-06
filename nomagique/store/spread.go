package store

import (
	"github.com/theapemachine/symm/nomagique/core"
)

/*
Spread hands over a retained run one value at a time.

Arity lives in time in this algebra, so something has to turn a value that
holds many into many yields, and that is the whole of what this does. It is
what lets an ordinary accumulator reduce a run: an Add shown a Spread folds
every value in it, without either of them knowing the other is there.

The run belongs to the caller. Spread reloads from its source when a new
caller starts, delivers until the run is exhausted, and ends with nil, so the
next caller is handed the run from the beginning rather than the remains of
somebody else's pass.
*/
type Spread struct {
	core.PrimitiveError
	source core.Primitive
	values []float64
	index  int
	caller core.Primitive
	open   bool
}

/*
NewSpread configures the Primitive whose values are handed over. It is
stepped, never drained, at construction: a Primitive is whole before it is
advanced, and the source is read when a run begins.
*/
func NewSpread(source core.Primitive) *Spread {
	return &Spread{
		source: source,
	}
}

/*
Next hands over the next value of the current run, and nil once the run is
spent. Whoever asks first begins a run, and anybody else asking begins their
own, which is what makes the same Spread readable by more than one stage.
*/
func (spread *Spread) Next(in core.Primitive) core.Primitive {
	if !spread.open || in != spread.caller {
		spread.load(in)
	}

	if spread.index >= len(spread.values) {
		spread.open = false

		return nil
	}

	spread.index++

	return core.From(spread.values[spread.index-1])
}

/*
Read surfaces the run being handed over for the boundary.
*/
func (spread *Spread) Read() any {
	return spread.values
}

/*
load takes the run from the source and starts a run for this caller. The
source is drained through Yield like any other incoming Primitive, so a
source that hands over one run and a source that hands over several are read
the same way, and neither has to announce which it is.
*/
func (spread *Spread) load(in core.Primitive) {
	gathered := core.Yield(
		core.From([]float64(nil)), spread.source,
		func(held, arriving []float64) []float64 {
			return append(held, arriving...)
		},
	)

	spread.Error(gathered.Error())

	spread.values = core.To[[]float64](gathered)
	spread.index = 0
	spread.caller = in
	spread.open = true
}
