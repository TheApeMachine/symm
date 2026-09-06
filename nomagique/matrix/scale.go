package matrix

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
	"github.com/theapemachine/symm/nomagique/vector"
)

// NewScale maps the same configured scalar multiplier over every row.
func NewScale(values, scale core.Primitive) core.Primitive {
	context := store.NewRetained(nil)
	return transport.NewPipe(
		store.NewRecord(transport.NewPipe(values, store.NewKey("matrix")), transport.NewPipe(scale, store.NewKey("scale"))),
		context,
		store.NewGet("matrix"),
		transport.NewSpread[[]float64](),
		transport.NewMap(vector.NewScale(transport.NewPipe(),
			transport.NewApply(store.NewGet("scale"), context))),
		transport.NewCollect[[]float64](),
	)
}
