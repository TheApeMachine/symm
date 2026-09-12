package store

import (
	container "container/ring"
	"errors"
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
RingCommand is one intent against a Ring. Write appends one value or one
nested child ring; WriteAt inserts at an offset slot relative to the current
element; Advance moves the parent to the next child; Len asks for the parent
length; Play asks for the current child's values in order (and advances the
parent afterwards, exactly as one playback run always did).
*/
type RingCommand[T any] struct {
	Write     *RingValue[T]
	WriteAt   *RingSlot[T]
	Advance   bool
	Len       bool
	ChildLen  bool
	Current   bool
	Play      bool
}

/*
RingValue is a value written into a parent ring, which is either one T or one
nested Ring primitive whose held container becomes the parent's element.
*/
type RingValue[T any] struct {
	Value *T
	Child core.Primitive
}

/*
RingSlot is a RingValue placed at an offset slot relative to the current
element.
*/
type RingSlot[T any] struct {
	Value RingValue[T]
	Slot  int
}

/*
RingResult carries the answer to one RingCommand.
*/
type RingResult[T any] struct {
	Len      int
	ChildLen int
	Values   []T
	Value    T
	Has      bool
}

/*
Ring owns a ring of rings. Writing builds it; Play plays one child sequence
per command, then steps the parent so the sequences replay in order and their
order never restarts. A ring grows to hold exactly what was written — a ring
sized up front would leave nil slots for anything the caller did not fill,
and those are not values a run can carry.
*/
type Ring[T any] struct {
	err    error
	parent *container.Ring
	child  *container.Ring
	played int
	out    RingResult[T]
}

/*
NewRing begins an empty ring primitive.
*/
func NewRing[T any]() core.Primitive {
	return &Ring[T]{}
}

/*
NewRingOver plays out a ring that is already built, which is what a caller
with its own container has.
*/
func NewRingOver[T any](parent *container.Ring) core.Primitive {
	return &Ring[T]{parent: parent}
}

func (op *Ring[T]) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			command := (*RingCommand[T])(arriving)
			op.out = RingResult[T]{}

			if !op.apply(command) {
				return
			}

			if command.Play {
				for index := range op.out.Values {
					held := op.out.Values[index]

					if !yield(unsafe.Pointer(&held)) {
						return
					}
				}

				continue
			}

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

/*
apply resolves one command against the ring, reporting false when the run
must stop. The first value written is the element the first Play reads,
because a ring has no beginning of its own, only the element a reader starts
at.
*/
func (op *Ring[T]) apply(command *RingCommand[T]) bool {
	intents := 0

	for _, held := range []bool{
		command.Write != nil, command.WriteAt != nil,
		command.Advance, command.Len, command.ChildLen, command.Current, command.Play,
	} {
		if held {
			intents++
		}
	}

	if intents != 1 {
		op.err = errors.Join(op.err, core.ErrShape)
		return false
	}

	switch {
	case command.Write != nil:
		op.link(op.element(*command.Write), 0, false)
	case command.WriteAt != nil:
		op.link(op.element(command.WriteAt.Value), command.WriteAt.Slot, true)
	case command.Advance:
		if op.parent != nil {
			op.parent = op.parent.Next()
		}
	case command.Len:
		if op.parent != nil {
			op.out.Len = op.parent.Len()
		}
	case command.ChildLen:
		if child, held := op.currentChild(); held {
			op.out.ChildLen = child.Len()
		}
	case command.Current:
		if op.parent != nil {
			if value, ok := op.parent.Value.(T); ok {
				op.out.Value, op.out.Has = value, true
			}
		}
	case command.Play:
		if !op.play() {
			return false
		}
	}

	return true
}

/*
currentChild reads the child ring at the parent's current position.
*/
func (op *Ring[T]) currentChild() (*container.Ring, bool) {
	if op.parent == nil {
		return nil, false
	}

	child, held := op.parent.Value.(*container.Ring)

	return child, held && child != nil
}

/*
play reads the current child sequence into the result, then steps the parent,
so the next Play begins at the next child.
*/
func (op *Ring[T]) play() bool {
	if op.parent == nil {
		return true
	}

	child, held := op.parent.Value.(*container.Ring)

	if !held || child == nil {
		return true
	}

	op.child, op.played = child, 0
	op.out.Values = make([]T, 0, child.Len())

	for op.played < op.child.Len() {
		value, ok := op.child.Value.(T)
		op.child, op.played = op.child.Next(), op.played+1

		if !ok {
			op.err = errors.Join(op.err, core.ErrShape)
			return false
		}

		op.out.Values = append(op.out.Values, value)
	}

	op.parent = op.parent.Next()

	return true
}

/*
element builds the container element one RingValue describes: a nested ring
primitive contributes the container it built, which is what makes a ring of
rings.
*/
func (op *Ring[T]) element(value RingValue[T]) *container.Ring {
	held := container.New(1)

	if value.Child != nil {
		if child, nested := value.Child.(*Ring[T]); nested && child.parent != nil {
			held.Value = child.parent
		}
	}

	if held.Value == nil && value.Value != nil {
		held.Value = *value.Value
	}

	return held
}

/*
link places one element. Appending links the new element in behind the
current one and steps back to the first — Prev is where a ring's last element
is, so writing there keeps the order the caller wrote in. A slotted write
inserts at the offset instead.
*/
func (op *Ring[T]) link(held *container.Ring, slot int, atSlot bool) {
	if op.parent == nil {
		op.parent = held

		return
	}

	if !atSlot {
		op.parent.Prev().Link(held)

		return
	}

	op.parent.Move(slot).Link(held)
}

func (op *Ring[T]) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}
