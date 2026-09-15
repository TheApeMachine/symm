package store

import (
	container "container/ring"
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
Ring owns a ring of rings. Writing builds it; Play plays one child sequence
per command, then steps the parent so the sequences replay in order and their
order never restarts. A ring grows to hold exactly what was written — a ring
sized up front would leave nil slots for anything the caller did not fill,
and those are not values a run can carry.
*/
type Ring[T any] struct {
	*core.PrimitiveError
	store     *container.Ring
	randomize bool
}

/*
NewRing begins an empty ring primitive.
*/
func NewRing[T any](n int, randomize bool) *Ring[T] {
	return &Ring[T]{
		PrimitiveError: core.NewPrimitiveError(),
		store:          container.New(n),
		randomize:      randomize,
	}
}

func (op *Ring[T]) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		query := data.Read[Query[T]](in)

		switch query.Action() {
		case data.ActionWrite:
			op.store = op.store.Next()
			op.store.Value = query.payload

			if !yield(unsafe.Pointer(&op.store.Value)) {
				return
			}
		case data.ActionRead:
			op.store = op.store.Next()

			if !yield(unsafe.Pointer(&op.store.Value)) {
				return
			}
		default:
			op.Error(core.ErrShape)
			return
		}
	}
}
