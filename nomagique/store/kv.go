package store

import (
	"sync"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
KV is the most generic form of holding values by name, and exists so that
nothing else in the system has to invent its own. Primitives that need to
carry a set of named values, rather than a single one, compose a KV instead
of growing map fields of their own.

It is deliberately not a configuration type, a cache, or a registry. Those
are all uses of a key-value store, and giving KV any of their vocabulary
would make it something more specific than the one thing it does.
*/
type KV struct {
	core.PrimitiveError
	values *sync.Map
}

/*
NewKV builds an empty store. The map is created here rather than lazily in
Next, so that a KV is usable the moment it exists and no step has to check
whether it has been initialised.
*/
func NewKV() *KV {
	return &KV{
		values: &sync.Map{},
	}
}

/*
Next folds an incoming store into this one, so that composing two KVs merges
them with the incoming values winning. Offered nothing, it answers with
itself, which is what a Primitive holding configuration does when asked what
it is: the caller needs the store, not a value out of it.
*/
func (kv *KV) Next(in core.Primitive) core.Primitive {
	if in == nil {
		return kv
	}

	incoming := core.To[*sync.Map](in)

	if incoming == nil {
		return kv
	}

	incoming.Range(func(key, value any) bool {
		kv.values.Store(key, value)

		return true
	})

	return kv
}

/*
Read surfaces the underlying store for the boundary.
*/
func (kv *KV) Read() any {
	return kv.values
}
