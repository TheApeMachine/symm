package causal

import (
	"github.com/theapemachine/symm/nomagique/collection"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

// NewBackdoor standardizes configured predictions over the observed rows after
// replacing the treatment coordinate. Fitting and predicting are Primitive
// connections, not a linear/nonlinear switch. The result is a model-dependent
// interventional estimate, not a claim that observational data identifies cause.
func NewBackdoor(fit, predict core.Primitive) core.Primitive {
	context := store.NewRetained(nil)
	return logic.NewGate(
		NewFeatures(),
		transport.NewPipe(
			store.NewRecord(transport.NewPipe(), transport.NewPipe(fit, store.NewKey("fit"))),
			context,
			store.NewRecord(
				transport.NewPipe(),
				transport.NewPipe(store.NewGet("fit"), store.NewGet("defined"), store.NewKey("defined")),
				transport.NewPipe(
					logic.NewGate(
						transport.NewPipe(store.NewGet("fit"), store.NewGet("defined")),
						transport.NewPipe(
							store.NewGet("rows"),
							transport.NewSpread[[]float64](),
							transport.NewMap(
								transport.NewPipe(
									collection.NewSet[float64](
										transport.NewApply(store.NewGet("treatment"), context),
										transport.NewApply(store.NewGet("level"), context),
									),
									store.NewKey("row"),
									store.NewKV[string](context),
									predict,
								),
							),
							equation.NewMean(),
						),
						equation.NewRatio[float64](store.NewConstant(core.From(0.0)), store.NewConstant(core.From(0.0))),
					),
					store.NewKey("expectation"),
				),
			),
		),
		logic.NewReject(core.ErrDomain),
	)
}
