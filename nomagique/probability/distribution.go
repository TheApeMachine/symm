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
	return transport.NewPipe(
		equation.NewSoftmax(),
		transport.NewCollect[float64](),
		store.NewRecord(
			transport.NewPipe(store.NewKey("probabilities")),
			transport.NewPipe(transport.NewSpread[float64](), equation.NewArgmax(), store.NewGet("index"), store.NewKey("winner")),
			transport.NewPipe(transport.NewSpread[float64](), equation.NewArgmax(), store.NewGet("value"), store.NewKey("confidence")),
			transport.NewPipe(transport.NewSpread[float64](), NewAmbiguity(), store.NewKey("ambiguity")),
		),
		store.NewRecord(
			transport.NewPipe(),
			transport.NewPipe(
				equation.NewDifference[float64](store.NewConstant(core.From(1.0)), store.NewGet("ambiguity")),
				store.NewKey("sharpness"),
			),
		),
	)
}
