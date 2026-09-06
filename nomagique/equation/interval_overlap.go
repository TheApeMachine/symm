package equation

import (
	"github.com/theapemachine/symm/nomagique/collection"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

// NewIntervalOverlap is (left.from < right.to) AND (right.from < left.to).
// Endpoints touching at a single instant do not overlap for (from,to] spans.
func NewIntervalOverlap() core.Primitive {
	return transport.NewPipe(
		transport.NewFan(
			transport.NewPipe(),
			transport.NewIO(
				NewLess[int64](
					transport.NewPipe(collection.NewAt[core.Primitive](transport.NewIO(core.From(0.0))), store.NewGet("from")),
					transport.NewPipe(collection.NewAt[core.Primitive](transport.NewIO(core.From(1.0))), store.NewGet("to")),
				),
				NewLess[int64](
					transport.NewPipe(collection.NewAt[core.Primitive](transport.NewIO(core.From(1.0))), store.NewGet("from")),
					transport.NewPipe(collection.NewAt[core.Primitive](transport.NewIO(core.From(0.0))), store.NewGet("to")),
				),
			),
		),
		logic.NewAnd(transport.NewIO(core.From(true))),
	)
}
