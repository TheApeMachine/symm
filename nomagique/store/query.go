package store

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
Query is the interrogation protocol every store answers. It carries a subject
(what is addressed, via Identify), an intent (what should happen, via
Action), and the value a write puts in place. The store answers into the same
query: a read fills Value, a write into an unstamped subject reports the ID
it was assigned.
*/
type Query struct {
	*core.PrimitiveError
	data.Identifiable
	data.Actionable
	subject data.Identifiable
	payload iter.Seq[unsafe.Pointer]
}

/*
NewQuery instantiates one interrogation; the subject's identity seeds the
query, so an unstamped subject asks for an append and a stamped one addresses
its slot.
*/
func NewQuery(
	subject data.Identifiable,
	action data.Actionable,
	payload ...iter.Seq[unsafe.Pointer],
) *Query {
	var seq iter.Seq[unsafe.Pointer]

	if len(payload) > 0 {
		seq = payload[0]
	}

	return &Query{
		PrimitiveError: core.NewPrimitiveError(),
		Identifiable:   subject,
		Actionable:     action,
		subject:        subject,
		payload:        seq,
	}
}

/*
Next ignores the inbound run and yields the query itself, so a query rides a
pipeline like any other payload.
*/
func (op *Query) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		if !yield(unsafe.Pointer(op)) {
			return
		}
	}
}

func (op *Query) Identity() int {
	if op.subject != nil {
		return op.subject.Identity()
	}

	return -1
}

func (op *Query) Identify(id int) data.Identifiable {
	if op.subject != nil {
		return op.subject.Identify(id)
	}

	return op
}
