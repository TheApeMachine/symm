package correlation

import (
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

// NewFisherEstimator transforms admissible scalar correlations before the
// configured causal estimator, then maps its baseline back with Tanh. Invalid
// observations do not advance the estimator and are visibly undefined. The
// configured estimator supplies recurrence; this composer keeps no second copy.
func NewFisherEstimator(estimator core.Primitive) core.Primitive {
	input := store.NewRetained(nil)
	return transport.NewMap(transport.NewPipe(
		input,
		logic.NewGate(equation.NewAll(
			equation.NewLess[float64](store.NewConstant(core.From(-1.0)), transport.NewPipe()),
			equation.NewLess[float64](transport.NewPipe(), store.NewConstant(core.From(1.0)))),
			transport.NewPipe(
				calculus.NewAtanh(transport.NewIO(core.From(0.0))), estimator,
				store.NewRecord(transport.NewPipe(),
					transport.NewPipe(store.NewConstant(core.From(true)), store.NewKey("defined")),
					transport.NewPipe(transport.NewApply(input, nil), store.NewKey("correlation")),
					transport.NewPipe(store.NewGet("baseline"), calculus.NewTanh(transport.NewIO(core.From(0.0))), store.NewKey("baseline")),
					transport.NewPipe(store.NewGet("residual"), store.NewKey("divergence")))),
			store.NewRecord(
				transport.NewPipe(transport.NewApply(input, nil), store.NewKey("correlation")),
				transport.NewPipe(store.NewConstant(core.From(false)), store.NewKey("defined")))),
	))
}
