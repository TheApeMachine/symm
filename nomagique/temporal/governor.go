package temporal

import (
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/collection"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

// NewGovernor composes an observation-driven capacity, retained tail and
// reduction. The controller yields a record containing capacity (float64).
// Neither the store nor reduction knows which policy selected the capacity.
func NewGovernor(controller, reduction core.Primitive) core.Primitive {
	history := store.NewRetained(core.From([]float64{}))
	capacity := store.NewRetained(core.From(2))
	return transport.NewMap(
		transport.NewFan(
			transport.NewPipe(),
			transport.NewIO(
				transport.NewPipe(
					controller,
					store.NewGet("capacity"),
					calculus.NewMaximum(transport.NewIO(core.From(2.0))),
					calculus.NewConvert[float64, int](),
					capacity,
					transport.NewDiscard(),
				),
				transport.NewPipe(
					collection.NewAppend[float64](history),
					collection.NewTail[float64](capacity),
					history,
					logic.NewGate(
						equation.NewGreater[float64](
							transport.NewPipe(transport.NewSpread[float64](), equation.NewCount()),
							store.NewConstant(core.From(1.0)),
						),
						transport.NewPipe(transport.NewSpread[float64](), reduction),
						store.NewConstant(core.From(0.0)),
					),
				),
			),
		),
	)
}
