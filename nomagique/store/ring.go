package store

import (
	container "container/ring"
	"iter"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Ring plays out a ring of rings. Each Next run is one child sequence. When that
child is spent the run ends, and the next Next begins at the parent's next
child. The parent loops, so the sequences replay and their order never restarts.
*/
type Ring[T any] struct {
	core.Base[T, T]
	parent *container.Ring
	child  *container.Ring
	played int
}

/*
NewRing begins an empty ring. Values are written into it in order, and the ring
grows to hold exactly what was written — a ring sized up front would leave nil
slots for anything the caller did not fill, and those are not values a run can
carry.
*/
func NewRing[T any]() *Ring[T] {
	return &Ring[T]{}
}

/*
Held is the container this ring built, so one ring can be written into another.
*/
func (op *Ring[T]) Held() *container.Ring { return op.parent }

/*
NewRingOver plays out a ring that is already built, which is what a caller with
its own container has.
*/
func NewRingOver[T any](parent *container.Ring) *Ring[T] {
	return &Ring[T]{parent: parent}
}

/*
Write appends one value, and leaves the ring pointing at what was written
first.

Position matters here: a ring has no beginning of its own, only the element a
reader starts at, so the first value written is the one the first run plays.
*/
func (op *Ring[T]) Write(value any) {
	held := container.New(1)

	/*
		A ring written into a ring is the container it built, not the primitive
		that built it. That is what makes a ring of rings: the parent's every
		element is a child ring, and Next reads it back as one.
	*/
	if child, nested := value.(interface{ Held() *container.Ring }); nested {
		held.Value = child.Held()
	}

	if held.Value == nil {
		held.Value = value
	}

	if op.parent == nil {
		op.parent = held

		return
	}
	// Link the new element in behind the current one, then step back to the
	// first: Prev is where a ring's last element is, so writing there keeps
	// the order the caller wrote in.
	op.parent.Prev().Link(held)
}

func (op *Ring[T]) Next(
	iter.Seq[core.Primitive[T, T]],
) iter.Seq[core.Primitive[T, T]] {
	return func(yield func(core.Primitive[T, T]) bool) {
		if op.parent == nil {
			return
		}

		child, held := op.parent.Value.(*container.Ring)

		if !held || child == nil {
			return
		}

		op.child, op.played = child, 0

		for op.played < op.child.Len() {
			value, ok := op.child.Value.(T)
			op.child, op.played = op.child.Next(), op.played+1

			if !ok {
				return
			}

			if !yield(op.Carrier(value)) {
				return
			}
		}

		op.parent = op.parent.Next()
	}
}

/*
Len returns the number of elements in the parent ring.
*/
func (op *Ring[T]) Len() int {
	if op.parent == nil {
		return 0
	}

	return op.parent.Len()
}

/*
ChildLen returns the length of the child ring at the current parent position.
*/
func (op *Ring[T]) ChildLen() int {
	if op.parent == nil {
		return 0
	}

	child, held := op.parent.Value.(*container.Ring)

	if !held || child == nil {
		return 0
	}

	return child.Len()
}

/*
CurrentChildValues returns all values from the child ring at the current parent position,
preserving their original sequence.
*/
func (op *Ring[T]) CurrentChildValues() []T {
	if op.parent == nil {
		return nil
	}

	child, held := op.parent.Value.(*container.Ring)

	if !held || child == nil {
		return nil
	}

	length := child.Len()

	if length == 0 {
		return nil
	}

	values := make([]T, 0, length)
	current := child

	for range length {
		if value, ok := current.Value.(T); ok {
			values = append(values, value)
		}

		current = current.Next()
	}

	return values
}

/*
Advance moves the parent ring forward to the next child sequence.
*/
func (op *Ring[T]) Advance() {
	if op.parent != nil {
		op.parent = op.parent.Next()
	}
}

/*
WriteAt inserts one value into the parent ring at an offset slot relative
to the current element.
*/
func (op *Ring[T]) WriteAt(value any, slot int) {
	held := container.New(1)

	if child, nested := value.(interface{ Held() *container.Ring }); nested {
		held.Value = child.Held()
	}

	if held.Value == nil {
		held.Value = value
	}

	if op.parent == nil {
		op.parent = held

		return
	}

	target := op.parent.Move(slot)
	target.Link(held)
}

/*
NextOffset plays the current child sequence starting from a specified offset.
When that child is spent, the parent advances to the next child and loops.
*/
func (op *Ring[T]) NextOffset(
	_ iter.Seq[core.Primitive[T, T]],
	offset int,
) iter.Seq[core.Primitive[T, T]] {
	return func(yield func(core.Primitive[T, T]) bool) {
		if op.parent == nil {
			return
		}

		child, held := op.parent.Value.(*container.Ring)

		if !held || child == nil {
			return
		}

		childLen := child.Len()

		if childLen == 0 {
			op.parent = op.parent.Next()

			return
		}

		if offset < 0 {
			offset = 0
		}

		if offset >= childLen {
			offset = childLen - 1
		}

		op.child = child.Move(offset)
		op.played = offset

		for op.played < childLen {
			value, ok := op.child.Value.(T)
			op.child, op.played = op.child.Next(), op.played + 1

			if !ok {
				return
			}

			if !yield(op.Carrier(value)) {
				return
			}
		}

		op.parent = op.parent.Next()
	}
}
