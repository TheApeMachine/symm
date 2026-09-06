package equation

import (
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

func NewBonferroni() core.Primitive {
	return transport.NewPipe(
		NewProduct[float64](store.NewGet("p"), store.NewGet("candidates")),
		calculus.NewMinimum(transport.NewIO(core.From(1.0))),
	)
}
