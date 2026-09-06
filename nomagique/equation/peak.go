package equation

import (
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/collection"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

// NewPeak selects the first maximum absolute y, retaining the entire point and
// its structural index. It is composition, not a profile-scanning helper.
func NewPeak() core.Primitive {
	return transport.NewPipe(
		transport.NewEnumerate(),
		logic.NewPick(
			transport.NewPipe(
				transport.NewFan(
					transport.NewPipe(),
					transport.NewIO(
						transport.NewPipe(
							transport.NewPipe(
								collection.NewAt[core.Primitive](transport.NewIO(core.From(1.0))),
								collection.NewAt[core.Primitive](transport.NewIO(core.From(1.0))),
							),
							store.NewGet("y"),
							logic.NewIsNaN(),
						),
						NewGreater[float64](
							transport.NewPipe(
								transport.NewPipe(
									collection.NewAt[core.Primitive](transport.NewIO(core.From(1.0))),
									collection.NewAt[core.Primitive](transport.NewIO(core.From(1.0))),
								),
								store.NewGet("y"),
								calculus.NewAbsolute(transport.NewIO(core.From(0.0))),
							),
							transport.NewPipe(
								transport.NewPipe(
									collection.NewAt[core.Primitive](transport.NewIO(core.From(0.0))),
									collection.NewAt[core.Primitive](transport.NewIO(core.From(1.0))),
								),
								store.NewGet("y"),
								calculus.NewAbsolute(transport.NewIO(core.From(0.0))),
							),
						),
					),
				),
				logic.NewOr(transport.NewIO(core.From(false))),
			),
		),
		transport.NewSpread[core.Primitive](),
		transport.NewMap(store.NewRecord(
			transport.NewPipe(collection.NewAt[core.Primitive](transport.NewIO(core.From(0.0))), store.NewKey("index")),
			transport.NewPipe(collection.NewAt[core.Primitive](transport.NewIO(core.From(1.0))), store.NewKey("point")),
		)),
	)
}
