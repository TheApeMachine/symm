package store

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
Query aliases core.Query for backwards-compatibility in store package.
*/
type Query[T, U any] = core.Query[T, U]

func NewQuery[T, U any](
	address core.Connectable[T], action core.Action,
) *Query[T, U] {
	return core.NewQuery[T, U](address, action)
}

func NewKeyQuery[U any](
	key *[]byte, action core.Action,
) *Query[*[]byte, U] {
	addr := transport.NewAddress[*[]byte]()
	addr.Identify(key)
	return core.NewQuery[*[]byte, U](addr, action)
}
