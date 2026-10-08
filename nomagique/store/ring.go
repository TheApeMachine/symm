package store

import (
	container "container/ring"
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Ring owns a ring of runs. Every inbound run is written as one child sequence
into the next slot and played through; a nil run plays the child sequence in
the next slot instead, so written sequences replay in order and their order
never restarts. Playing a slot that was never written is ErrNotHeld: an empty
slot is not a value a run can carry.
*/
type Ring struct {
	*core.PrimitiveError
	store *container.Ring
}

/*
NewRing begins a ring primitive of n slots.
*/
func NewRing(n int) *Ring {
	return &Ring{
		PrimitiveError: core.NewPrimitiveError(),
		store:          container.New(n),
	}
}

func (op *Ring) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		if in == nil {
			op.store.Do(func(value any) {
				if !yield(value.(unsafe.Pointer)) {
					return
				}
			})

			return
		}

		for arriving := range in {
			op.store.Value = arriving
			op.store = op.store.Next()
		}
	}
}
