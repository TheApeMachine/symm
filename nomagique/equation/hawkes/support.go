package hawkes

import (
	"github.com/theapemachine/symm/nomagique/arithmetic"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

// NewSupport reduces a configured response over events strictly before horizon
// on the selected side. Equal-time events do not excite one another. The whole
// retained history may precede origin; origin is not a support cutoff.
func NewSupport(side, response core.Primitive) core.Primitive {
	context := store.NewRetained(nil)
	return transport.NewPipe(
		context,
		store.NewGet("events"),
		transport.NewSpread[core.Primitive](),
		transport.NewMap(
			logic.NewGate(
				equation.NewAll(
					equation.NewEqual[float64](store.NewGet("side"), transport.NewApply(side, context)),
					equation.NewLess[float64](store.NewGet("at"), transport.NewApply(store.NewGet("horizon"), context)),
				),
				transport.NewPipe(
					store.NewRecord(
						transport.NewPipe(),
						transport.NewPipe(transport.NewApply(store.NewGet("beta"), context), store.NewKey("beta")),
						transport.NewPipe(
							equation.NewDifference[float64](transport.NewApply(store.NewGet("horizon"), context), store.NewGet("at")),
							store.NewKey("age"),
						),
					),
					response,
				),
				transport.NewDiscard(),
			),
		),
		arithmetic.NewAdd[float64](transport.NewIO(core.From(0.0))),
	)
}
