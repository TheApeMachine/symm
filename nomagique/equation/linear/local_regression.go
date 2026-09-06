package linear

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

// NewLocalRegression uses elapsed seconds from an exact int64 nanosecond
// origin as x and the observed value as y. The first origin is retained. Like
// the supplied implementation, this is cumulative; its old horizon argument
// never restricted evidence and no new retention policy is silently added.
func NewLocalRegression() core.Primitive {
	memory := store.NewRetained(core.From(map[string]core.Primitive{}))
	origin := transport.NewPipe(
		store.NewKV[string](memory),
		logic.NewGate(
			store.NewHas("origin"),
			transport.NewPipe(),
			store.NewRecord(transport.NewPipe(), transport.NewPipe(store.NewGet("at"), store.NewKey("origin"))),
		),
		transport.NewFan(
			transport.NewPipe(),
			transport.NewIO(
				transport.NewPipe(
					store.NewRecord(transport.NewPipe(store.NewGet("origin"), store.NewKey("origin"))),
					memory,
					transport.NewDiscard(),
				),
				transport.NewPipe(),
			),
		),
	)
	return transport.NewMap(
		transport.NewPipe(
			origin,
			store.NewRecord(
				transport.NewPipe(),
				transport.NewPipe(equation.NewElapsed(store.NewGet("origin"), store.NewGet("at")), store.NewKey("x")),
				transport.NewPipe(store.NewGet("value"), store.NewKey("y")),
			),
			NewRegressionMoments(),
			NewRegressionSummary(),
		),
	)
}
