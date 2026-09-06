package vector

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

// NewScale evaluates the vector and scalar once, then maps multiplication.
// The vector expression yields a []float64 payload; scale yields one scalar.
func NewScale(values, scale core.Primitive) core.Primitive {
	record := store.NewRetained(nil)
	return transport.NewPipe(
		store.NewRecord(
			transport.NewPipe(values, store.NewKey("values")), transport.NewPipe(scale, store.NewKey("scale"))),
		record,
		store.NewGet("values"),
		transport.NewSpread[float64](),
		transport.NewMap(equation.NewProduct[float64](
			transport.NewPipe(), transport.NewApply(store.NewGet("scale"), record))),
		transport.NewCollect[float64](),
	)
}
