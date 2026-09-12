package store

import (
	"errors"
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/data"
)

/*
Query is the interrogation protocol every store answers. It carries a subject
(what is addressed, via Identify), an intent (what should happen, via
Action), and the value a write puts in place. The store answers into the same
query: a read fills Value, a write into an unstamped subject reports the ID
it was assigned.
*/
type Query[T any] struct {
	data.Identifiable[T]
	data.Actionable
	err     error
	subject data.Identifiable[T]
	payload []T
}

/*
NewQuery instantiates one interrogation; the subject's identity seeds the
query, so an unstamped subject asks for an append and a stamped one addresses
its slot.
*/
func NewQuery[T any](
	subject data.Identifiable[T], action data.Actionable, payload ...T,
) *Query[T] {
	return &Query[T]{
		Identifiable: subject,
		Actionable:   action,
		subject:      subject,
		payload:      payload,
	}
}

/*
Next ignores the inbound run and yields the query itself, so a query rides a
pipeline like any other payload.
*/
func (op *Query[T]) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		if !yield(unsafe.Pointer(op)) {
			return
		}
	}
}

func (op *Query[T]) Identity() int {
	return op.subject.Identity()
}

func (op *Query[T]) Identify(id int) data.Identifiable[T] {
	return op.subject.Identify(id)
}

func (op *Query[T]) First() T {
	return op.payload[0]
}

func (op *Query[T]) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}
