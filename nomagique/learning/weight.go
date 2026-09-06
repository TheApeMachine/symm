package learning

import (
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

// NewTrustWeight composes the source's residual-range trust update. No clipping
// is added to the trust recurrence; rate greater than one retains the original
// extrapolating behavior. Invalid inputs fail before entering retained state.
func NewTrustWeight() core.Primitive {
	memory := store.NewRetained(
		core.From(
			map[string]core.Primitive{
				"count": core.From(0.0), "minimum": core.From(0.0), "maximum": core.From(0.0), "prev": core.From(0.0), "trust": core.From(1.0), "rate": core.From(0.0),
			},
		),
	)
	state := transport.NewPipe(store.NewKV[string](memory), memory)
	return transport.NewMap(
		logic.NewGate(
			equation.NewValidPair(),
			transport.NewPipe(
				store.NewRecord(
					transport.NewPipe(),
					transport.NewPipe(equation.NewDifference[float64](store.NewGet("actual"), store.NewGet("predicted")), store.NewKey("residual")),
				),
				state,
				equation.NewResidualSpan(),
				logic.NewGate(
					equation.NewGreater[float64](store.NewGet("count"), store.NewConstant(core.From(1.0))),
					logic.NewGate(
						equation.NewGreater[float64](store.NewGet("span"), store.NewConstant(core.From(0.0))),
						transport.NewPipe(
							store.NewRecord(
								transport.NewPipe(),
								transport.NewPipe(
									equation.NewRatio[float64](
										transport.NewPipe(store.NewGet("residual"), calculus.NewAbsolute(transport.NewIO(core.From(0.0)))),
										store.NewGet("span"),
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
								transport.NewPipe(store.NewGet("predicted"), store.NewKey("prev")),
							),
						),
						logic.NewReject(core.ErrDomain),
					),
					transport.NewPipe(),
				),
				store.NewRecord(transport.NewPipe(), transport.NewPipe(store.NewGet("trust"), store.NewKey("value"))),
				state,
			),
			logic.NewReject(core.ErrDomain),
		),
	)
}
