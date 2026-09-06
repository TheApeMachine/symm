package probability

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

// NewDistribution composes a simplex and its named readouts. Malformed logits
// fail explicitly, as they do in Softmax. A published record contains values,
// not references to temporary retention slots which a later run could change.
func NewDistribution() core.Primitive {
	return logic.NewGate(
		equation.NewAll(
			equation.NewGreater[float64](equation.NewCount(), store.NewConstant(core.From(0.0))),
			transport.NewMapReduce(logic.NewFinite(), logic.NewAnd(transport.NewIO(core.From(true))))),
		transport.NewPipe(
			equation.NewSoftmax(), transport.NewCollect[float64](),
			store.NewRecord(
				transport.NewPipe(store.NewKey("probabilities")),
				transport.NewPipe(transport.NewSpread[float64](), equation.NewArgmax(),
					store.NewRecord(
						transport.NewPipe(store.NewGet("index"), store.NewKey("winner")),
						transport.NewPipe(store.NewGet("value"), store.NewKey("confidence")))),
				transport.NewPipe(transport.NewSpread[float64](), NewAmbiguity(), store.NewKey("ambiguity"))),
			store.NewRecord(transport.NewPipe(),
				transport.NewPipe(equation.NewDifference[float64](store.NewConstant(core.From(1.0)), store.NewGet("ambiguity")), store.NewKey("sharpness")))),
		logic.NewReject(core.ErrDomain),
	)
}
