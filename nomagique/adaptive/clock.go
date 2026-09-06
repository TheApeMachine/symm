package adaptive

import (
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

// NewClock normalizes |value| by the estimator's inclusive mean and applies
// its configured pace. Volume time is obtained by placing Absolute before this
// graph; signed interarrival behavior does not require a ClockType switch.
func NewClock(moments, pace core.Primitive) core.Primitive {
	return transport.NewPipe(
		moments,
		transport.NewMap(
			equation.NewProduct[float64](
				logic.NewGate(
					equation.NewGreater[float64](store.NewGet("mean"), store.NewConstant(core.From(0.0))),
					equation.NewRatio[float64](
						transport.NewPipe(store.NewGet("value"), calculus.NewAbsolute(transport.NewIO(core.From(0.0)))),
						store.NewGet("mean"),
					),
					store.NewConstant(core.From(1.0)),
				),
				pace,
			),
		),
	)
}
