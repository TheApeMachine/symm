package core

import (
	"iter"
	"unsafe"
)

/*
Query addresses a store with identity T and a payload stream of U.
Entity optionally scopes the address. The payload remains borrowed until the
run completes; First reads U, independently of the address representation.
*/
type Query[T, U any] struct {
	*PrimitiveError
	Connectable[T]
	Action  Action
	Payload iter.Seq[unsafe.Pointer]
}

/*
NewQuery binds a subject and intent. Without a subject, Address is supplied
directly by the caller; its zero value has no special missing-address meaning.
*/
func NewQuery[T, U any](
	address Connectable[T], action Action,
) *Query[T, U] {
	query := &Query[T, U]{
		PrimitiveError: NewPrimitiveError(),
		Action:         action,
		Connectable:    address,
	}

	return query
}

/*
Next takes in and yields a payload for the query. Together with the Action, this
helps the receiving Primitive decide how to behave. For example, in case of a
Read action, the payload can be the query itself. In case of a Write action, the
payload can be a new value.
*/
func (query *Query[T, U]) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		if query.Error() != nil {
			return
		}

		query.Payload = in

		if !yield(unsafe.Pointer(query)) {
			return
		}
	}
}

func (query *Query[T, U]) Identity() T {
	if query.Connectable == nil {
		var zero T
		return zero
	}

	return query.Connectable.Identity()
}

func (query *Query[T, U]) Identify(identity T) Identifiable[T] {
	if query.Connectable == nil {
		return query
	}

	return query.Connectable.Identify(identity)
}

func (query *Query[T, U]) Error(errs ...error) error {
	return query.PrimitiveError.Error(errs...)
}

func (query *Query[T, U]) Connect(primitive Primitive) {
	if query.Connectable != nil {
		query.Connectable.Connect(primitive)
	}
}
