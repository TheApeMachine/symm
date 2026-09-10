package store

import (
	container "container/ring"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Ring plays out a ring of rings.

The child is one recorded sequence and is handed over a position at a time.
When it has been played through, the run ends and the parent advances, so the
next run begins at the next sequence rather than running through the seam
between them as if the two were one. The parent loops, so the sequences replay
endlessly and their order never restarts.
*/
type Ring struct {
	core.PrimitiveError
	parent  *container.Ring
	child   *container.Ring
	played  int
	current core.Primitive
}

/* NewRing plays out the ring of rings the configured state carries. */
func NewRing(state core.Primitive) *Ring {
	return &Ring{parent: core.To[*container.Ring](state)}
}

func (ring *Ring) Next(core.Primitive) core.Primitive {
	if ring.parent == nil {
		return nil
	}

	if ring.child == nil {
		ring.parent = ring.parent.Next()
		child, held := ring.parent.Value.(*container.Ring)

		if !held || child == nil {
			return nil
		}
		ring.child, ring.played = child, 0
	}

	// The child has been played through: the run ends here, and the next one
	// begins on the parent's next sequence.
	if ring.played == ring.child.Len() {
		ring.child = nil

		return nil
	}
	value, held := ring.child.Value.(core.Primitive)
	ring.child, ring.played = ring.child.Next(), ring.played+1

	if !held {
		return nil
	}
	ring.current = value

	return value
}

func (ring *Ring) Read() any { return core.To[any](ring.current) }
