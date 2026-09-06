package equation

import (
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

// NewPriorUpdate is the source's normalized reliability-weighted recurrence.
// Incoming authority is strictly positive here; the enclosing composition
// handles zero-authority completions without adding evidence.
func NewPriorUpdate() core.Primitive {
	return logic.NewGate(
		NewEqual[float64](store.NewGet("weight"), store.NewConstant(core.From(0.0))),
		store.NewRecord(
			transport.NewPipe(),
			transport.NewPipe(store.NewGet("value"), store.NewKey("mean")),
			transport.NewPipe(store.NewGet("authority"), store.NewKey("weight")),
			transport.NewPipe(store.NewConstant(core.From(1.0)), store.NewKey("support")),
			transport.NewPipe(store.NewConstant(core.From(0.0)), store.NewKey("moment")),
		),
		transport.NewPipe(
			store.NewRecord(
				transport.NewPipe(),
				transport.NewPipe(NewSum[float64](store.NewGet("weight"), store.NewGet("authority")), store.NewKey("total")),
				transport.NewPipe(NewDifference[float64](store.NewGet("value"), store.NewGet("mean")), store.NewKey("difference")),
			),
			store.NewRecord(
				transport.NewPipe(),
				transport.NewPipe(NewRatio[float64](store.NewGet("weight"), store.NewGet("total")), store.NewKey("retained_fraction")),
				transport.NewPipe(NewRatio[float64](store.NewGet("authority"), store.NewGet("total")), store.NewKey("incoming_fraction")),
			),
			store.NewRecord(
				transport.NewPipe(),
				transport.NewPipe(
					NewRatio[float64](
						store.NewConstant(core.From(1.0)),
						NewSum[float64](
							NewRatio[float64](
								transport.NewPipe(store.NewGet("retained_fraction"), calculus.NewSquare(transport.NewIO(core.From(0.0)))),
								store.NewGet("support"),
							),
							transport.NewPipe(store.NewGet("incoming_fraction"), calculus.NewSquare(transport.NewIO(core.From(0.0)))),
						),
					),
					store.NewKey("support"),
				),
				transport.NewPipe(
					NewSum[float64](
						NewProduct[float64](store.NewGet("retained_fraction"), store.NewGet("moment")),
						NewProduct[float64](
							NewProduct[float64](store.NewGet("retained_fraction"), store.NewGet("incoming_fraction")),
							transport.NewPipe(store.NewGet("difference"), calculus.NewSquare(transport.NewIO(core.From(0.0)))),
						),
					),
					store.NewKey("moment"),
				),
				transport.NewPipe(
					NewSum[float64](store.NewGet("mean"), NewProduct[float64](store.NewGet("incoming_fraction"), store.NewGet("difference"))),
					store.NewKey("mean"),
				),
				transport.NewPipe(store.NewGet("total"), store.NewKey("weight")),
			),
		),
	)
}
