package store

import (
	container "container/ring"
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
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

func (ring *Ring[T]) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		next, stop := iter.Pull(in)
		defer stop()

		arriving, ok := next()
		if !ok {
			return
		}

		query := (*Query[int, T])(arriving)

		switch query.Action {
		case core.Write:
			ring.store = ring.store.Next()
			payload, ok := next()
			if !ok {
				ring.Error(core.ErrShape)
				return
			}
			ring.store.Value = *(*T)(payload)

			if !yield(unsafe.Pointer(&ring.store.Value)) {
				return
			}
		case core.Read:
			ring.store = ring.store.Next()

			if !yield(unsafe.Pointer(&ring.store.Value)) {
				return
			}
		default:
			ring.Error(core.ErrShape)
			return
		}
	}
}
