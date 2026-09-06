package learning

import (
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

// NewDirectionalTarget composes a finite nonnegative deadband and sign of a delta.
// deadband is evaluated on the input record. To use a separately updated store
// as read-only configuration, pass transport.NewApply(retained, nil).
func NewDirectionalTarget(deadband core.Primitive) core.Primitive {
	return transport.NewPipe(
		store.NewRecord(transport.NewPipe(NewDeltaTarget(), store.NewKey("delta")), transport.NewPipe(deadband, store.NewKey("deadband"))),
		logic.NewGate(
			equation.NewAll(
				transport.NewPipe(store.NewGet("deadband"), logic.NewFinite()),
				equation.NewLessEqual[float64](store.NewConstant(core.From(0.0)), store.NewGet("deadband")),
			),
			logic.NewGate(
				equation.NewLessEqual[float64](
					transport.NewPipe(store.NewGet("delta"), calculus.NewAbsolute(transport.NewIO(core.From(0.0)))),
					store.NewGet("deadband"),
				),
				store.NewConstant(core.From(0.0)),
				transport.NewPipe(store.NewGet("delta"), calculus.NewSign(transport.NewIO(core.From(0.0)))),
			),
			logic.NewReject(core.ErrDomain)),
	)
}
