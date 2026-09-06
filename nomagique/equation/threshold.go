package equation

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/store"
)

// NewThreshold multiplies dispersion by a configured coefficient expression.
// The source's no-dispersion threshold of one is explicit at this level.
func NewThreshold(coefficient core.Primitive) core.Primitive {
	return logic.NewGate(
		NewGreater[float64](store.NewGet("dispersion"), store.NewConstant(core.From(0.0))),
		NewProduct[float64](store.NewGet("dispersion"), coefficient),
		store.NewConstant(core.From(1.0)),
	)
}
