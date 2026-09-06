package store

import (
	"github.com/theapemachine/symm/nomagique/core"
)

/*
First is the leading member of a pair.

Pairs travel through this system as one value — a return and the one before
it, a weight and what it weighs, an offset and what was found there — so
something has to take them apart again, and taking one member is the smallest
operation that does. It is generic because which member is wanted has nothing
to do with what the members are.
*/
type First[T any] struct {
	core.PrimitiveError
	current core.Primitive
}

/*
NewFirst configures the value held before anything has been shown.
*/
func NewFirst[T any](state core.Primitive) *First[T] {
	return &First[T]{
		current: state,
	}
}

/*
Next applies the projection to every pair the incoming Primitive yields and
holds the leading member of the last of them. The fold is the operation:
Yield owns the draining, and nothing is collected or converted back into Go.
*/
func (first *First[T]) Next(in core.Primitive) core.Primitive {
	gathered := core.Yield(
		core.From([2]T{}), in, func(_, pair [2]T) [2]T {
			first.current = core.From(pair[0])

			return pair
		},
	)

	first.current.Error(gathered.Error())

	return first.current
}

/*
Read surfaces the member for the boundary.
*/
func (first *First[T]) Read() any {
	return first.current.Read()
}
