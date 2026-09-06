package causal

import (
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/collection"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

// NewCounterfactual composes abduction, intervention and prediction, preserving
// the factual residual as in the supplied table. fit and the two prediction
// connections may be arbitrary compatible graphs. Precision is the source's
// reconstruction audit weight 1/(1+abs(noise)), not a causal probability.
func NewCounterfactual(fit, factual, intervened core.Primitive) core.Primitive {
	context := store.NewRetained(nil)
	prediction := transport.NewPipe(
		store.NewRecord(
			transport.NewPipe(),
			transport.NewPipe(
				store.NewRecord(transport.NewPipe(), transport.NewPipe(store.NewGet("actual"), store.NewKey("row"))),
				factual,
				store.NewKey("factual_prediction"),
			),
			transport.NewPipe(
				store.NewRecord(
					transport.NewPipe(),
					transport.NewPipe(
						store.NewGet("actual"),
						collection.NewSet[float64](
							transport.NewApply(store.NewGet("treatment"), context),
							transport.NewApply(store.NewGet("level"), context),
						),
						store.NewKey("row"),
					),
				),
				intervened,
				store.NewKey("intervened_prediction"),
			),
		),
		store.NewRecord(
			transport.NewPipe(),
			transport.NewPipe(
				equation.NewDifference[float64](
					transport.NewPipe(store.NewGet("actual"), collection.NewAt[float64](transport.NewApply(store.NewGet("target"), context))),
					store.NewGet("factual_prediction"),
				),
				store.NewKey("noise"),
			),
		),
		store.NewRecord(
			transport.NewPipe(),
			transport.NewPipe(
				equation.NewSum[float64](store.NewGet("intervened_prediction"), store.NewGet("noise")),
				store.NewKey("counterfactual"),
			),
			transport.NewPipe(
				equation.NewRatio[float64](
					store.NewConstant(core.From(1.0)),
					equation.NewSum[float64](
						store.NewConstant(core.From(1.0)),
						transport.NewPipe(store.NewGet("noise"), calculus.NewAbsolute(transport.NewIO(core.From(0.0)))),
					),
				),
				store.NewKey("precision"),
			),
			transport.NewPipe(store.NewConstant(core.From(true)), store.NewKey("defined")),
		),
	)
	undefined := store.NewRecord(
		transport.NewPipe(),
		transport.NewPipe(store.NewConstant(core.From(false)), store.NewKey("defined")),
		transport.NewPipe(
			equation.NewRatio[float64](store.NewConstant(core.From(0.0)), store.NewConstant(core.From(0.0))),
			store.NewKey("noise"),
		),
		transport.NewPipe(
			equation.NewRatio[float64](store.NewConstant(core.From(0.0)), store.NewConstant(core.From(0.0))),
			store.NewKey("counterfactual"),
		),
		transport.NewPipe(
			equation.NewRatio[float64](store.NewConstant(core.From(0.0)), store.NewConstant(core.From(0.0))),
			store.NewKey("precision"),
		),
	)
	return logic.NewGate(
		NewFeatures(),
		transport.NewPipe(
			store.NewRecord(transport.NewPipe(), transport.NewPipe(fit, store.NewKey("fit"))),
			context,
			logic.NewGate(transport.NewPipe(store.NewGet("fit"), store.NewGet("defined")), prediction, undefined),
		),
		logic.NewReject(core.ErrDomain),
	)
}
