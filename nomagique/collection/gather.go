package collection

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

// NewGather selects positions supplied by an index-stream expression. At owns
// index validation; retention owns the captured collection and current index.
func NewGather[T any](indices core.Primitive) core.Primitive {
	values := store.NewRetained(nil)
	index := store.NewRetained(nil)
	return transport.NewPipe(
		values,
		transport.NewApply(
			transport.NewPipe(
				transport.NewMap(transport.NewPipe(index, transport.NewApply(NewAt[T](index), values))),
				transport.NewCollect[T](),
			),
			transport.NewApply(indices, values),
		),
	)
}
