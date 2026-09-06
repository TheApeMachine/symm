package hawkes

import (
	"github.com/theapemachine/symm/nomagique/arithmetic"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

// NewScore sums a supplied per-event derivative on one target side.
// It consumes the previously computed responses, never the live event source.
func NewScore(side, response core.Primitive) core.Primitive {
	context := store.NewRetained(nil)
	return transport.NewPipe(
		context,
		store.NewGet("scored"),
		transport.NewSpread[core.Primitive](),
		transport.NewMap(
			logic.NewGate(
				equation.NewEqual[float64](store.NewGet("side"), transport.NewApply(side, context)),
				response,
				transport.NewDiscard(),
			),
		),
		arithmetic.NewAdd[float64](transport.NewIO(core.From(0.0))),
	)
}
