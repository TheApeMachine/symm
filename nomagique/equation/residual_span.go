package equation

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

// NewResidualSpan reproduces the supplied calibration range update. Its count
// stays at one until the first distinct residual, then counts every update.
// It is therefore the source's state counter, not a count of observations.
func NewResidualSpan() core.Primitive {
	return transport.NewPipe(
		logic.NewGate(
			NewEqual[float64](store.NewGet("count"), store.NewConstant(core.From(0.0))),
			store.NewRecord(
				transport.NewPipe(),
				transport.NewPipe(store.NewGet("residual"), store.NewKey("minimum")),
				transport.NewPipe(store.NewGet("residual"), store.NewKey("maximum")),
				transport.NewPipe(store.NewConstant(core.From(1.0)), store.NewKey("count")),
				transport.NewPipe(store.NewGet("predicted"), store.NewKey("prev")),
			),
			transport.NewPipe(),
		),
		logic.NewGate(
			NewGreater[float64](store.NewGet("count"), store.NewConstant(core.From(1.0))),
			store.NewRecord(
				transport.NewPipe(),
				transport.NewPipe(NewMinimum(store.NewGet("minimum"), store.NewGet("residual")), store.NewKey("minimum")),
				transport.NewPipe(NewMaximum(store.NewGet("maximum"), store.NewGet("residual")), store.NewKey("maximum")),
				transport.NewPipe(NewSum[float64](store.NewGet("count"), store.NewConstant(core.From(1.0))), store.NewKey("count")),
			),
			transport.NewPipe(),
		),
		logic.NewGate(
			NewAll(
				NewEqual[float64](store.NewGet("count"), store.NewConstant(core.From(1.0))),
				transport.NewPipe(
					NewEqual[float64](store.NewGet("residual"), store.NewGet("minimum")),
					logic.NewNot(transport.NewIO(core.From(false))),
				),
			),
			store.NewRecord(
				transport.NewPipe(),
				transport.NewPipe(NewMinimum(store.NewGet("minimum"), store.NewGet("residual")), store.NewKey("minimum")),
				transport.NewPipe(NewMaximum(store.NewGet("maximum"), store.NewGet("residual")), store.NewKey("maximum")),
				transport.NewPipe(store.NewConstant(core.From(2.0)), store.NewKey("count")),
			),
			transport.NewPipe(),
		),
		store.NewRecord(
			transport.NewPipe(),
			transport.NewPipe(NewDifference[float64](store.NewGet("maximum"), store.NewGet("minimum")), store.NewKey("span")),
		),
	)
}
