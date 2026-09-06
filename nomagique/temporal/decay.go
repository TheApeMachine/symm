package temporal

import (
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
	"math"
)

// NewDecay wires input -> clock -> shape, multiplying the original input by the
// resulting retention factor. No clock means infinite elapsed time; the default
// linear shape therefore extinguishes a finite input. Configuration is not
// evaluated by construction, and each slot can be an arbitrary composition.
func NewDecay(clock, shape core.Primitive) core.Primitive {
	if clock == nil {
		clock = store.NewConstant(core.From(math.Inf(1)))
	}
	if shape == nil {
		shape = transport.NewPipe(
			equation.NewDifference[float64](store.NewConstant(core.From(1.0)), transport.NewPipe()),
			calculus.NewMaximum(transport.NewIO(core.From(0.0))),
		)
	}
	return transport.NewMap(equation.NewProduct[float64](transport.NewPipe(), transport.NewPipe(clock, shape)))
}
