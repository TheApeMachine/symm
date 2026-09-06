package store

import (
	"github.com/theapemachine/symm/nomagique/core"
)

/*
Spread hands over a retained run one value at a time.

Arity lives in time in this algebra, so something has to turn a value holding
many into many yields, and that is the whole of what this does. It is what
lets an ordinary accumulator reduce a run, and what lets an operation be
applied to every member of one: without it a run can only be gathered and
picked apart imperatively, which is not composition.

It is generic in what it hands over because handing over has nothing to do
with what is being handed. A run of numbers, of observations, of returns, or
of anything a later domain invents travels the same way.

The run belongs to the caller. Spread reloads from its source when a new
caller starts, delivers until the run is spent, and ends with nil, so the
next caller is handed the run from the beginning rather than the remains of
somebody else's pass.
*/
type Spread[T any] struct {
	core.PrimitiveError
	source core.Primitive
	values []T
	index  int
	caller core.Primitive
	open   bool
}

/*
NewSpread configures the Primitive whose run is handed over. It is not
drained here: a Primitive is whole before it is advanced, and the run is
taken when a caller begins one.
*/
func NewSpread[T any](source core.Primitive) *Spread[T] {
	return &Spread[T]{
		source: source,
	}
}

/*
Next hands over the next value of the current run, and nil once the run is
spent. Whoever asks first begins a run, and anybody else asking begins their
own, which is what makes the same Spread readable by more than one stage.
*/
func (spread *Spread[T]) Next(in core.Primitive) core.Primitive {
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
func (spread *Spread[T]) Read() any {
	return spread.values
}

/*
load takes the run from the source and starts a run for this caller. The fold
keeps what arrives, so the run is never converted back into Go to be counted
or indexed; only the source's own values are held.
*/
func (spread *Spread[T]) load(in core.Primitive) {
	spread.values = spread.values[:0]

	gathered := core.Yield(
		core.From([]T(nil)), spread.source,
		func(_, arriving []T) []T {
			spread.values = append(spread.values, arriving...)

			return arriving
		},
	)

	spread.Error(gathered.Error())

	spread.index = 0
	spread.caller = in
	spread.open = true
}
