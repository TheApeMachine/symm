package hawkes

import (
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/transport"
)

// NewKernel is exp(-beta*age). Rate and age are Primitive expressions;
// no event, side, storage or observation-window policy belongs to the kernel.
func NewKernel(beta, age core.Primitive) core.Primitive {
	return transport.NewPipe(
		transport.NewPipe(equation.NewProduct[float64](beta, age), calculus.NewNegate(transport.NewIO(core.From(0.0)))),
		calculus.NewExp(transport.NewIO(core.From(0.0))),
	)
}
