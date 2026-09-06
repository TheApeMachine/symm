package probability

import (
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

// NewAmbiguity divides entropy by the entropy of an equal-mass distribution.
// A one-member distribution has zero ambiguity by definition.
func NewAmbiguity() core.Primitive {
	return logic.NewGate(
		transport.NewPipe(
			transport.NewFan(transport.NewPipe(), transport.NewIO(equation.NewCount(), store.NewConstant(core.From(1.0)))),
			transport.NewCollect[float64](),
			logic.NewGreater[float64](),
		),
		equation.NewRatio[float64](
			transport.NewPipe(equation.NewNormalize(), equation.NewEntropy()),
			transport.NewPipe(equation.NewCount(), calculus.NewLog(transport.NewIO(core.From(0.0)))),
		),
		store.NewConstant(core.From(0.0)),
	)
}
