package store

import (
	"github.com/theapemachine/symm/nomagique/core"
)

/*
Query is a primitive that either selects data from a store,
inserts data into a store, or updates data in a store.

To select data, make sure it only has a selector and no data.
To insert data, make sure it has only data and no selector.
To update data, make sure it has both a selector and data.

It hands its parts over one at a time, selector first, so a store learns what is
being asked of it before it is shown anything to write. That ordering is the
whole protocol: a store that has been handed a selector and then nothing was
asked to read, and one handed data after it is being asked to write there.
*/
type Query[T any] struct {
	core.PrimitiveError
	current core.Primitive
}

func NewQuery[T any](state core.Primitive) *Query[T] {
	return &Query[T]{current: state}
}

/*
Next hands over the next part of the question and nil once it is fully asked,
so the run ends and the same question can be asked again.
*/
func (query *Query[T]) Next(in core.Primitive) core.Primitive {
	return core.Yield(
		query.current,
		in,
		func(held T, arriving T) T { return arriving },
		query,
	)
}

/* Read surfaces what is being asked about. */
func (query *Query[T]) Read() any { return query.current }
