package adaptive

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

// NewEnvelope uses the supplied moment and width expressions. Choice of a
// Gaussian or Chebyshev coefficient is topology, not an EnvelopeType switch.
func NewEnvelope(moments, coefficient core.Primitive) core.Primitive {
	return transport.NewPipe(
		moments,
		transport.NewMap(
			logic.NewGate(
				equation.NewAll(
					equation.NewGreater[float64](store.NewGet("count"), store.NewConstant(core.From(1.0))),
					equation.NewGreater[float64](store.NewGet("dispersion"), store.NewConstant(core.From(0.0))),
				),
				transport.NewPipe(
					store.NewRecord(
						transport.NewPipe(),
						transport.NewPipe(
							equation.NewProduct[float64](store.NewGet("dispersion"), coefficient), store.NewKey("margin")),
					),
					equation.NewBound(
						store.NewGet("value"),
						equation.NewDifference[float64](store.NewGet("mean"), store.NewGet("margin")),
						equation.NewSum[float64](store.NewGet("mean"), store.NewGet("margin")),
					),
				),
				store.NewGet("value"),
			),
		),
	)
}
