package equation

import (
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

// NewPriorSummary preserves the distinction between completion count, support,
// input authority and outcome magnitude. Flags distinguish undefined variance
// from a measured zero. This is not a calibrated success probability.
func NewPriorSummary() core.Primitive {
	return transport.NewPipe(
		store.NewRecord(
			transport.NewPipe(),
			transport.NewPipe(store.NewConstant(core.From(false)), store.NewKey("defined")),
			transport.NewPipe(store.NewConstant(core.From(false)), store.NewKey("variance_defined")),
			transport.NewPipe(store.NewConstant(core.From(0.0)), store.NewKey("variance")),
			transport.NewPipe(store.NewConstant(core.From(0.0)), store.NewKey("maturity")),
			transport.NewPipe(store.NewConstant(core.From(0.0)), store.NewKey("evidence_authority")),
			transport.NewPipe(store.NewConstant(core.From(0.0)), store.NewKey("authority")),
		),
		logic.NewGate(
			NewGreater[float64](store.NewGet("weight"), store.NewConstant(core.From(0.0))),
			transport.NewPipe(
				store.NewRecord(
					transport.NewPipe(),
					transport.NewPipe(store.NewConstant(core.From(true)), store.NewKey("defined")),
					transport.NewPipe(NewRatio[float64](store.NewGet("weight"), store.NewGet("support")), store.NewKey("evidence_authority")),
				),
				logic.NewGate(
					NewGreater[float64](store.NewGet("support"), store.NewConstant(core.From(1.0))),
					transport.NewPipe(
						store.NewRecord(
							transport.NewPipe(),
							transport.NewPipe(store.NewConstant(core.From(true)), store.NewKey("variance_defined")),
							transport.NewPipe(
								NewProduct[float64](
									store.NewGet("moment"),
									NewRatio[float64](store.NewGet("support"),
										NewDifference[float64](store.NewGet("support"), store.NewConstant(core.From(1.0)))),
								),
								store.NewKey("variance"),
							),
							transport.NewPipe(
								NewRatio[float64](NewDifference[float64](store.NewGet("support"), store.NewConstant(core.From(1.0))), store.NewGet("support")),
								store.NewKey("maturity"),
							),
							transport.NewPipe(store.NewGet("mean"), calculus.NewSquare(transport.NewIO(core.From(0.0))), store.NewKey("power")),
						),
						store.NewRecord(
							transport.NewPipe(),
							transport.NewPipe(NewSum[float64](store.NewGet("power"), store.NewGet("variance")), store.NewKey("total_power")),
						),
						logic.NewGate(
							NewGreater[float64](store.NewGet("total_power"), store.NewConstant(core.From(0.0))),
							store.NewRecord(
								transport.NewPipe(),
								transport.NewPipe(
									NewProduct[float64](
										NewProduct[float64](store.NewGet("maturity"), store.NewGet("evidence_authority")),
										NewRatio[float64](store.NewGet("power"), store.NewGet("total_power")),
									),
									store.NewKey("authority"),
								),
							),
							transport.NewPipe(),
						),
					),
					transport.NewPipe(),
				),
			),
			store.NewRecord(
				transport.NewPipe(),
				transport.NewPipe(store.NewConstant(core.From(0.0)), store.NewKey("mean")),
				transport.NewPipe(store.NewConstant(core.From(0.0)), store.NewKey("support")),
			),
		),
	)
}
