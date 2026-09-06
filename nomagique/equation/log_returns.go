package equation

import (
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/collection"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

// NewLogReturns composes adjacent pairing, temporal admission and log difference.
// Input observations are KV records (at: int64 nanoseconds, value: float64).
// Every output interval remains KV (from, to, value); no Observation DTO exists.
func NewLogReturns() core.Primitive {
	return transport.NewPipe(
		transport.NewWindow(2, 1),
		transport.NewMap(
			logic.NewGate(
				NewLess[int64](
					transport.NewPipe(collection.NewAt[core.Primitive](transport.NewIO(core.From(0.0))), store.NewGet("at")),
					transport.NewPipe(collection.NewAt[core.Primitive](transport.NewIO(core.From(1.0))), store.NewGet("at")),
				),
				store.NewRecord(
					transport.NewPipe(collection.NewAt[core.Primitive](transport.NewIO(core.From(0.0))), store.NewGet("at"), store.NewKey("from")),
					transport.NewPipe(collection.NewAt[core.Primitive](transport.NewIO(core.From(1.0))), store.NewGet("at"), store.NewKey("to")),
					transport.NewPipe(
						NewDifference[float64](
							transport.NewPipe(
								collection.NewAt[core.Primitive](transport.NewIO(core.From(1.0))),
								store.NewGet("value"),
								calculus.NewLog(transport.NewIO(core.From(0.0))),
							),
							transport.NewPipe(
								collection.NewAt[core.Primitive](transport.NewIO(core.From(0.0))),
								store.NewGet("value"),
								calculus.NewLog(transport.NewIO(core.From(0.0))),
							),
						),
						store.NewKey("value"),
					),
				),
				logic.NewReject(core.ErrShape),
			),
		),
	)
}
