package store

import (
	"github.com/theapemachine/symm/nomagique/core"
)

/*
Pairs hands over the members of a run two at a time, each with the one before
it.

Every difference is an operation on adjacent members: a return is the change
between two observations, a gap is the time between them, a slope is the rise
between them. Without this, each of those has to walk a gathered run itself
and index into it, which is the same operation written out again in every
place that needs it.

The pairs overlap, because the second member of one is the first member of
the next: a run of four values is three differences, not two.

It is generic in what it pairs, and it holds only the value it has already
handed forward, since that is all pairing needs. It steps its source one
value at a time, because taking one value at a time is what pairing adjacent
members is, and reads each into the pair it hands on: the same unwrapping
Yield performs when it folds a run.
*/
type Pairs[T any] struct {
	core.PrimitiveError
	source   core.Primitive
	previous core.Primitive
	caller   core.Primitive
	open     bool
}

/*
NewPairs configures the Primitive whose run is paired.
*/
func NewPairs[T any](source core.Primitive) *Pairs[T] {
	return &Pairs[T]{
		source: source,
	}
}

/*
Next hands over the next value of the run together with the one before it,
and nil once the run is spent. A new caller starts a new run, so the first
pair it is handed is the first two members rather than wherever somebody else
had reached.
*/
func (pairs *Pairs[T]) Next(in core.Primitive) core.Primitive {
	if !pairs.open || in != pairs.caller {
		pairs.previous, pairs.caller, pairs.open = pairs.source.Next(in), in, true
	}

	if pairs.previous == nil {
		pairs.open = false

		return nil
	}

	current := pairs.source.Next(in)

	if current == nil {
		pairs.open = false

		return nil
	}

	paired := core.From([2]T{
		core.To[T](pairs.previous), core.To[T](current),
	})

	pairs.previous = current

	return paired
}

/*
Read surfaces the value most recently handed forward for the boundary, and
nothing before a run has begun.
*/
func (pairs *Pairs[T]) Read() any {
	if pairs.previous == nil {
		return nil
	}

	return pairs.previous.Read()
}
