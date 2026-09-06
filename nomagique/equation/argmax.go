package equation

import (
	"github.com/theapemachine/symm/nomagique/collection"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

// NewArgmax preserves a winning value's ordinal through comparison and selection.
// Strict comparison keeps the first equal maximum. No hidden profile engine.
func NewArgmax() core.Primitive {
	return transport.NewPipe(
		transport.NewEnumerate(),
		logic.NewPick(
			transport.NewPipe(
				transport.NewFan(
					transport.NewPipe(),
					transport.NewIO(
						transport.NewPipe(
							collection.NewAt[core.Primitive](transport.NewIO(core.From(1.0))),
							collection.NewAt[core.Primitive](transport.NewIO(core.From(1.0))),
						),
						transport.NewPipe(
							collection.NewAt[core.Primitive](transport.NewIO(core.From(0.0))),
							collection.NewAt[core.Primitive](transport.NewIO(core.From(1.0))),
						),
					),
				),
				transport.NewCollect[float64](),
				logic.NewGreater[float64](),
			),
		),
		transport.NewSpread[core.Primitive](),
		transport.NewMap(store.NewRecord(
			transport.NewPipe(collection.NewAt[core.Primitive](transport.NewIO(core.From(0.0))), store.NewKey("index")),
			transport.NewPipe(collection.NewAt[core.Primitive](transport.NewIO(core.From(1.0))), store.NewKey("value")),
		)),
	)
}
