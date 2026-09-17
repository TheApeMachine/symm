package store

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	sequence "github.com/theapemachine/symm/nomagique/data/sequence"
)

/*
Query addresses a store with identity T and a payload stream of U.
Entity optionally scopes the address. The payload remains borrowed until the
run completes; First reads U, independently of the address representation.
*/
type Query[T, U any] struct {
	*core.PrimitiveError
	data.Actionable
	Entity  string
	Address T

	subject   core.Identifiable[T]
	payload   iter.Seq[unsafe.Pointer]
	peerLimit int
	sequence  int64
}

/*
NewQuery binds a subject and intent. Without a subject, Address is supplied
directly by the caller; its zero value has no special missing-address meaning.
*/
func NewQuery[T, U any](
	subject core.Identifiable[T], action data.Actionable, payload ...iter.Seq[unsafe.Pointer],
) *Query[T, U] {
	var stream iter.Seq[unsafe.Pointer]

	if len(payload) > 0 {
		stream = payload[0]
	}

	query := &Query[T, U]{
		PrimitiveError: core.NewPrimitiveError(), Actionable: action,
		subject: subject, payload: stream, peerLimit: -1, sequence: -1,
	}

	if subject != nil {
		query.Address = subject.Identity()
	}

	return query
}

/*
Next yields an addressed request. Write and Execute bind the upstream run
when no payload was supplied, preserving the computation or input being used.
Other requests finish the upstream run before issuing their one command.
A query has one active consumer. Bound input is borrowed until yield returns,
then released; the same query can be reused for the next run.
*/
func (query *Query[T, U]) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		if query.payload == nil && (query.Action() == data.ActionWrite || query.Action() == data.ActionExecute) {
			query.payload = in
			defer func() { query.payload = nil }()
			yield(unsafe.Pointer(query))
			return
		}

		if in != nil {
			for range in {
			}
		}

		if !yield(unsafe.Pointer(query)) {
			return
		}
	}
}

/*
NewKeyQuery binds a radix address; the caller owns its bytes until the run ends.
*/
func NewKeyQuery[U any](key *[]byte, action data.Actionable, payload ...iter.Seq[unsafe.Pointer]) *Query[*[]byte, U] {
	query := NewQuery[*[]byte, U](nil, action, payload...)
	query.Address = key
	return query
}

func (query *Query[T, U]) Identity() T {
	if query.subject != nil {
		return query.subject.Identity()
	}

	return query.Address
}

func (query *Query[T, U]) Identify(identity T) core.Identifiable[T] {
	query.Address = identity

	if query.subject != nil {
		query.subject.Identify(identity)
	}

	return query
}

func (query *Query[T, U]) First() U {
	return sequence.Read[U](query.payload)
}

func (query *Query[T, U]) PeerLimit() int {
	return query.peerLimit
}

func (query *Query[T, U]) SetPeerLimit(limit int) *Query[T, U] {
	query.peerLimit = limit
	return query
}

/*
SetSequence addresses the committed Disruptor slot instead of latest peer state.
*/
func (query *Query[T, U]) SetSequence(sequence int64) *Query[T, U] {
	query.sequence = sequence
	return query
}
