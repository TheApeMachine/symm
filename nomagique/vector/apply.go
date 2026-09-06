package vector

import (
	"github.com/theapemachine/symm/nomagique/collection"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

// NewApply aligns one incoming value with each configured operation and
// forwards the operation's complete output run. Zip checks cardinality; Fan
// invokes the selected endpoint. No new invocation mechanism is necessary.
// Operations retain their identity between runs, so each coordinate may own
// an independent recurrence. A multi-output endpoint remains multi-output.
func NewApply(operations core.Primitive) core.Primitive {
	pair := store.NewRetained(nil)
	value := transport.NewApply(
		collection.NewAt[core.Primitive](transport.NewIO(core.From(0.0))), pair,
	)
	operation := transport.NewApply(
		collection.NewAt[core.Primitive](transport.NewIO(core.From(1.0))), pair,
	)
	return transport.NewPipe(
		transport.NewZip(transport.NewPipe(), operations),
		transport.NewMap(transport.NewPipe(pair, transport.NewFan(value, operation))),
	)
}
