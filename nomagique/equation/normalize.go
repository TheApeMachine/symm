package equation

import (
	"github.com/theapemachine/symm/nomagique/arithmetic"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

// NewNormalize captures the total once and divides each original value by it.
// Zero totals remain mathematically undefined, not a fabricated distribution.
func NewNormalize() core.Primitive {
	total := store.NewRetained(core.From(0.0))
	return transport.NewPipe(
		transport.NewFan(
			transport.NewPipe(),
			transport.NewIO(
				transport.NewPipe(arithmetic.NewAdd[float64](transport.NewIO(core.From(0.0))), total, transport.NewDiscard()),
				transport.NewPipe(),
			),
		),
		transport.NewMap(NewRatio[float64](transport.NewPipe(), transport.NewApply(total, nil))),
	)
}
