package learning

import (
	"github.com/theapemachine/symm/nomagique/arithmetic"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/equation/rls"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/matrix"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

// NewRLSSum predicts a sum of future feature rows from a retained posterior.
// Shared coefficient covariance is evaluated on the summed design; independent
// observation noise is counted once per row. The posterior connection is never
// trained or replaced by this query.
func NewRLSSum(posterior core.Primitive) core.Primitive {
	return transport.NewPipe(
		store.NewRecord(
			transport.NewApply(posterior, nil),
			transport.NewPipe(transport.NewSpread[[]float64](), equation.NewCount(), store.NewKey("rows")),
			transport.NewPipe(
				transport.NewSpread[[]float64](),
				transport.NewMap(
					transport.NewPipe(
						transport.NewFan(transport.NewPipe(), transport.NewIO(store.NewConstant(core.From(1.0)), transport.NewSpread[float64]())),
						transport.NewCollect[float64](),
					),
				),
				transport.NewCollect[[]float64](),
				matrix.NewTranspose[float64](),
				transport.NewSpread[[]float64](),
				transport.NewMap(transport.NewPipe(transport.NewSpread[float64](), arithmetic.NewAdd[float64](transport.NewIO(core.From(0.0))))),
				transport.NewCollect[float64](),
				store.NewKey("design"),
			),
		),
		logic.NewGate(
			equation.NewGreater[float64](store.NewGet("rows"), store.NewConstant(core.From(0.0))),
			rls.NewPrediction(store.NewGet("rows")),
			logic.NewReject(core.ErrShape),
		),
	)
}
