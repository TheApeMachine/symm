package equation

import (
	"github.com/theapemachine/symm/nomagique/arithmetic"
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/transport"
)

// NewEnergy composes the sum of squared incoming scalar values.
func NewEnergy() core.Primitive {
	return transport.NewMapReduce(
		calculus.NewSquare(transport.NewIO(core.From(0.0))),
		arithmetic.NewAdd[float64](transport.NewIO(core.From(0.0))),
	)
}
