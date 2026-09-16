package store

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	sequence "github.com/theapemachine/symm/nomagique/data/sequence"
)

/*
Query is the interrogation protocol every store answers. It carries a subject
(what is addressed, via Identify), an intent (what should happen, via
Action), and the value a write puts in place. The store answers into the same
query: a read fills Value, a write into an unstamped subject reports the ID
it was assigned.
*/
type Query[T any] struct {
	*core.PrimitiveError

	data.Identifiable[T]
	data.Actionable

	subject   data.Identifiable[T]
	payload   iter.Seq[unsafe.Pointer]
	peerLimit int
	sequence  int64
	key       *[]byte
}

/*
NewQuery instantiates one interrogation; the subject's identity seeds the
query, so an unstamped subject asks for an append and a stamped one addresses
its slot.
*/
func NewQuery[T any](
	subject data.Identifiable[T], action data.Actionable, payload ...iter.Seq[unsafe.Pointer],
) *Query[T] {
	var seq iter.Seq[unsafe.Pointer]

	if len(payload) > 0 {
		seq = payload[0]
	}

	return &Query[T]{PrimitiveError: core.NewPrimitiveError(), Identifiable: subject,
		Actionable: action,
		subject:    subject,
		payload:    seq,
		peerLimit:  -1,
		sequence:   -1,
	}
}

/*
Next yields an addressed request. Write and Execute bind the upstream run
when no payload was supplied, preserving the computation or input being used.
Other requests finish the upstream run before issuing their one command.
*/
func (query *Query[T]) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		if query.payload == nil && (query.Action() == data.ActionWrite || query.Action() == data.ActionExecute) {
			request := *query
			request.payload = in
			yield(unsafe.Pointer(&request))
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

// NewKeyQuery binds a radix address; the caller owns its key until the run ends.
func NewKeyQuery[T any](key *[]byte, action data.Actionable, payload ...iter.Seq[unsafe.Pointer]) *Query[T] {
	query := NewQuery[T](nil, action, payload...)
	query.key = key
	return query
}

func (query *Query[T]) Identity() int {
	if query.subject != nil {
		return query.subject.Identity()
	}

	return -1
}

func (query *Query[T]) Identify(id int) data.Identifiable[T] {
	if query.subject != nil {
		return query.subject.Identify(id)
	}

	return query
}

func (query *Query[T]) First() T {
	return sequence.Read[T](query.payload)
}

func (query *Query[T]) PeerLimit() int {
	return query.peerLimit
}

func (query *Query[T]) SetPeerLimit(limit int) *Query[T] {
	query.peerLimit = limit
	return query
}

// SetSequence addresses the committed Disruptor slot instead of latest peer state.
func (query *Query[T]) SetSequence(sequence int64) *Query[T] {
	query.sequence = sequence
	return query
}
