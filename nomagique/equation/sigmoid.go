package equation

import (
	"github.com/theapemachine/symm/nomagique/arithmetic"
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/transport"
)

// NewSigmoid is a composition, including mapping over multiple inputs.
func NewSigmoid() core.Primitive {
	return transport.NewMap(
		transport.NewPipe(
			calculus.NewNegate(transport.NewIO(core.From(0.0))),
			calculus.NewExp(transport.NewIO(core.From(0.0))),
			arithmetic.NewAdd[float64](transport.NewIO(core.From(1.0))),
			calculus.NewReciprocal(transport.NewIO(core.From(0.0))),
		),
	)
}
