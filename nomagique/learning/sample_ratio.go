package learning

import (
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

// NewSampleRatio preserves the supplied calibration ratio and observed-range
// ceiling. Those are model policies, not statistical identities. They are
// explicit compositions over the shared residual-span update.
func NewSampleRatio() core.Primitive {
	memory := store.NewRetained(
		core.From(
			map[string]core.Primitive{
				"count": core.From(0.0), "minimum": core.From(0.0), "maximum": core.From(0.0), "prev": core.From(0.0), "peak_ratio": core.From(0.0),
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
				store.NewRecord(
					transport.NewPipe(),
					transport.NewPipe(
						logic.NewGate(
							equation.NewLess[float64](store.NewGet("actual"), store.NewGet("predicted")),
							equation.NewSum[float64](
								store.NewConstant(core.From(1.0)),
								equation.NewRatio[float64](store.NewGet("actual"), store.NewGet("predicted")),
							),
							equation.NewRatio[float64](store.NewGet("actual"), store.NewGet("predicted")),
						),
						store.NewKey("ratio"),
					),
					transport.NewPipe(
						logic.NewGate(
							equation.NewGreater[float64](store.NewGet("span"), store.NewConstant(core.From(0.0))),
							equation.NewSum[float64](
								store.NewConstant(core.From(1.0)),
								equation.NewRatio[float64](store.NewConstant(core.From(1.0)), store.NewGet("span")),
							),
							equation.NewSum[float64](
								store.NewConstant(core.From(1.0)),
								equation.NewRatio[float64](
									store.NewConstant(core.From(1.0)),
									transport.NewPipe(store.NewGet("prev"), calculus.NewAbsolute(transport.NewIO(core.From(0.0)))),
								),
							),
						),
						store.NewKey("ceiling"),
					),
				),
				logic.NewGate(
					equation.NewAny(
						equation.NewLessEqual[float64](store.NewGet("predicted"), store.NewGet("actual")),
						equation.NewLessEqual[float64](store.NewConstant(core.From(0.0)), store.NewGet("ratio")),
					),
					transport.NewPipe(
						store.NewRecord(
							transport.NewPipe(),
							transport.NewPipe(
								logic.NewGate(
									equation.NewGreater[float64](store.NewGet("ratio"), store.NewGet("ceiling")),
									store.NewGet("ceiling"),
									store.NewGet("ratio"),
								),
								store.NewKey("value"),
							),
						),
						store.NewRecord(
							transport.NewPipe(),
							transport.NewPipe(
								transport.NewPipe(
									transport.NewFan(transport.NewPipe(), transport.NewIO(store.NewGet("peak_ratio"), store.NewGet("value"))),
									calculus.NewMaximum(transport.NewIO(core.From(0.0))),
								),
								store.NewKey("peak_ratio"),
							),
							transport.NewPipe(store.NewGet("predicted"), store.NewKey("prev")),
						),
						state,
					),
					logic.NewReject(core.ErrDomain),
				),
			),
			logic.NewReject(core.ErrDomain),
		),
	)
}
