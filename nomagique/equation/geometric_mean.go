package equation

import (
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/transport"
)

// NewGeometricMean composes exp(mean(log(x))). Zero and invalid-domain behavior
// comes from those operations; no alternate product or hand-written loop exists.
func NewGeometricMean() core.Primitive {
	return transport.NewPipe(
		transport.NewMap(calculus.NewLog(transport.NewIO(core.From(0.0)))),
		NewMean(),
		calculus.NewExp(transport.NewIO(core.From(0.0))),
	)
}
