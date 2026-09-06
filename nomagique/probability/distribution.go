package probability

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

// NewDistribution composes a simplex and its named readouts. There is no second
// Collection interface and no accessors that bypass Primitive delivery.
func NewDistribution() core.Primitive {
	probabilities := store.NewRetained(core.From([]float64{}))
	ambiguity := store.NewRetained(core.From(0.0))
	return transport.NewPipe(
		equation.NewSoftmax(),
		transport.NewCollect[float64](),
		probabilities,
		transport.NewFan(
			transport.NewPipe(),
			transport.NewIO(
				transport.NewPipe(transport.NewSpread[float64](), NewAmbiguity(), ambiguity, transport.NewDiscard()),
				transport.NewPipe(),
			),
		),
		store.NewRecord(
			transport.NewPipe(store.NewKey("probabilities")),
			transport.NewPipe(transport.NewSpread[float64](), equation.NewArgmax(), store.NewGet("index"), store.NewKey("winner")),
			transport.NewPipe(transport.NewSpread[float64](), equation.NewArgmax(), store.NewGet("value"), store.NewKey("confidence")),
			transport.NewPipe(transport.NewApply(ambiguity, nil), store.NewKey("ambiguity")),
			transport.NewPipe(
				equation.NewDifference[float64](store.NewConstant(core.From(1.0)), transport.NewApply(ambiguity, nil)),
				store.NewKey("sharpness"),
			),
		),
	)
}
