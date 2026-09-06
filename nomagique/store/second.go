package store

import (
	"github.com/theapemachine/symm/nomagique/core"
)

/*
Second is the trailing member of a pair, and the counterpart of First.
*/
type Second[T any] struct {
	core.PrimitiveError
	current core.Primitive
}

/*
NewSecond configures the value held before anything has been shown.
*/
func NewSecond[T any](state core.Primitive) *Second[T] {
	return &Second[T]{
		current: state,
	}
}

/*
Next applies the projection to every pair the incoming Primitive yields and
holds the trailing member of the last of them.
*/
func (second *Second[T]) Next(in core.Primitive) core.Primitive {
	gathered := core.Yield(
		core.From([2]T{}), in, func(_, pair [2]T) [2]T {
			second.current = core.From(pair[1])

			return pair
		},
	)

	second.current.Error(gathered.Error())

	return second.current
}

/*
Read surfaces the member for the boundary.
*/
func (second *Second[T]) Read() any {
	return second.current.Read()
}
