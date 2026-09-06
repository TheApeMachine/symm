package hawkes

import (
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

// NewEventLikelihood evaluates event log intensities on (origin,horizon].
// Responses retain per-event supports for the score composition. Captured
// history is replayed explicitly; this reference topology is O(N*N), not the
// optimized chronological excitation recurrence from the original package.
func NewEventLikelihood() core.Primitive {
	context := store.NewRetained(nil)
	return transport.NewPipe(
		context,
		store.NewGet("events"),
		transport.NewSpread[core.Primitive](),
		transport.NewMap(
			logic.NewGate(
				equation.NewAll(
					equation.NewGreater[float64](store.NewGet("at"), transport.NewApply(store.NewGet("origin"), context)),
					equation.NewLessEqual[float64](store.NewGet("at"), transport.NewApply(store.NewGet("horizon"), context)),
				),
				transport.NewPipe(
					store.NewKV[string](context),
					store.NewRecord(transport.NewPipe(), transport.NewPipe(store.NewGet("at"), store.NewKey("horizon"))),
					NewIntensity(store.NewGet("side")),
					logic.NewGate(
						equation.NewAll(
							equation.NewGreater[float64](store.NewGet("intensity"), store.NewConstant(core.From(0.0))),
							transport.NewPipe(store.NewGet("intensity"), logic.NewFinite()),
						),
						store.NewRecord(
							transport.NewPipe(),
							transport.NewPipe(
								transport.NewPipe(store.NewGet("intensity"), calculus.NewLog(transport.NewIO(core.From(0.0)))),
								store.NewKey("log_intensity"),
							),
						),
						logic.NewReject(core.ErrDomain),
					),
				),
				transport.NewDiscard(),
			),
		),
		transport.NewCollect[core.Primitive](),
	)
}
