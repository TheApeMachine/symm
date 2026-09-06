package learning

import (
	"github.com/theapemachine/symm/nomagique/algo"
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

// NewForecast composes the supplied multiplicative forecast-scale learner.
// Welford owns the residual moments. Storage owns trust/scale; Mix supplies both
// recurrences. The current residual participates in its source-defined surprise
// statistic; this is not mislabeled as a strictly prior standardized residual.
func NewForecast() core.Primitive {
	memory := store.NewRetained(core.From(map[string]core.Primitive{"trust": core.From(1.0), "scale": core.From(1.0), "rate": core.From(0.0)}))
	state := transport.NewPipe(store.NewKV[string](memory), memory)
	return transport.NewMap(
		logic.NewGate(
			equation.NewValidPair(),
			transport.NewPipe(
				store.NewRecord(
					transport.NewPipe(),
					transport.NewPipe(equation.NewDifference[float64](store.NewGet("actual"), store.NewGet("predicted")), store.NewKey("residual")),
				),
				store.NewRecord(
					transport.NewPipe(),
					transport.NewPipe(transport.NewPipe(store.NewGet("residual"), algo.NewWelford()), store.NewKey("moments")),
				),
				state,
				logic.NewGate(
					equation.NewGreater[float64](transport.NewPipe(store.NewGet("moments"), store.NewGet("count")), store.NewConstant(core.From(1.0))),
					transport.NewPipe(
						store.NewRecord(
							transport.NewPipe(),
							transport.NewPipe(
								logic.NewGate(
									equation.NewGreater[float64](
										transport.NewPipe(store.NewGet("moments"), store.NewGet("variance")),
										store.NewConstant(core.From(0.0)),
									),
									equation.NewRatio[float64](
										transport.NewPipe(
											equation.NewDifference[float64](store.NewGet("residual"), transport.NewPipe(store.NewGet("moments"), store.NewGet("mean"))),
											calculus.NewAbsolute(transport.NewIO(core.From(0.0))),
										),
										transport.NewPipe(
											transport.NewPipe(store.NewGet("moments"), store.NewGet("variance")),
											calculus.NewSqrt(transport.NewIO(core.From(0.0))),
										),
									),
									store.NewConstant(core.From(0.0)),
								),
								store.NewKey("rate"),
							),
						),
						store.NewRecord(
							transport.NewPipe(),
							transport.NewPipe(
								transport.NewPipe(
									store.NewRecord(
										transport.NewPipe(store.NewGet("trust"), store.NewKey("left")),
										transport.NewPipe(
											transport.NewPipe(
												equation.NewDifference[float64](store.NewConstant(core.From(1.0)), store.NewGet("rate")),
												calculus.NewMaximum(transport.NewIO(core.From(0.0))),
											),
											store.NewKey("right"),
										),
										transport.NewPipe(store.NewGet("rate"), store.NewKey("weight")),
									),
									equation.NewMix(),
								),
								store.NewKey("trust"),
							),
						),
						store.NewRecord(
							transport.NewPipe(),
							transport.NewPipe(
								equation.NewProduct[float64](
									store.NewGet("rate"),
									equation.NewDifference[float64](store.NewConstant(core.From(1.0)), store.NewGet("trust")),
								),
								store.NewKey("learning_rate"),
							),
							transport.NewPipe(
								transport.NewPipe(
									equation.NewDifference[float64](
										transport.NewPipe(
											transport.NewPipe(store.NewGet("actual"), calculus.NewAbsolute(transport.NewIO(core.From(0.0)))),
											calculus.NewLog(transport.NewIO(core.From(0.0))),
										),
										transport.NewPipe(
											transport.NewPipe(store.NewGet("predicted"), calculus.NewAbsolute(transport.NewIO(core.From(0.0)))),
											calculus.NewLog(transport.NewIO(core.From(0.0))),
										),
									),
									calculus.NewExp(transport.NewIO(core.From(0.0))),
								),
								store.NewKey("target_scale"),
							),
						),
						store.NewRecord(
							transport.NewPipe(),
							transport.NewPipe(
								transport.NewPipe(
									store.NewRecord(
										transport.NewPipe(store.NewGet("scale"), store.NewKey("left")),
										transport.NewPipe(store.NewGet("target_scale"), store.NewKey("right")),
										transport.NewPipe(store.NewGet("learning_rate"), store.NewKey("weight")),
									),
									equation.NewMix(),
								),
								store.NewKey("scale"),
							),
						),
					),
					transport.NewPipe(),
				),
				store.NewRecord(
					transport.NewPipe(),
					transport.NewPipe(store.NewGet("scale"), store.NewKey("value")),
					transport.NewPipe(transport.NewPipe(store.NewGet("moments"), store.NewGet("count")), store.NewKey("count")),
					transport.NewPipe(transport.NewPipe(store.NewGet("moments"), store.NewGet("count")), store.NewKey("weight_count")),
				),
				logic.NewGate(transport.NewPipe(store.NewGet("scale"), logic.NewFinite()), state, logic.NewReject(core.ErrDomain)),
			),
			logic.NewReject(core.ErrDomain),
		),
	)
}
