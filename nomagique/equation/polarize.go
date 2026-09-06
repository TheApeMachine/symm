package equation

import (
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

// NewPolarize splits a signed value into nonnegative components and normalizes
// against a configured scale expression. The result preserves all components.
func NewPolarize(scale core.Primitive) core.Primitive {
	return transport.NewMap(
		transport.NewPipe(
			store.NewRecord(
				transport.NewPipe(store.NewConstant(core.From(0.0)), store.NewKey("alpha_normalized")),
				transport.NewPipe(store.NewConstant(core.From(0.0)), store.NewKey("beta_normalized")),
				transport.NewPipe(scale, store.NewKey("scale")),
				transport.NewPipe(calculus.NewMaximum(transport.NewIO(core.From(0.0))), store.NewKey("alpha")),
				transport.NewPipe(
					calculus.NewNegate(transport.NewIO(core.From(0.0))),
					calculus.NewMaximum(transport.NewIO(core.From(0.0))),
					store.NewKey("beta"),
				),
			),
			logic.NewGate(
				NewGreater[float64](store.NewGet("scale"), store.NewConstant(core.From(0.0))),
				store.NewRecord(
					transport.NewPipe(),
					transport.NewPipe(
						NewRatio[float64](store.NewGet("alpha"), NewSum[float64](store.NewGet("alpha"), store.NewGet("scale"))),
						store.NewKey("alpha_normalized"),
					),
					transport.NewPipe(
						NewRatio[float64](store.NewGet("beta"), NewSum[float64](store.NewGet("beta"), store.NewGet("scale"))),
						store.NewKey("beta_normalized"),
					),
				),
				transport.NewPipe(),
			),
			store.NewRecord(
				transport.NewPipe(),
				transport.NewPipe(
					NewDifference[float64](
						store.NewGet("alpha_normalized"), store.NewGet("beta_normalized")),
					store.NewKey("value"),
				),
			),
		),
	)
}
