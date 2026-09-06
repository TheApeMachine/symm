package equation

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

// NewBound captures the value and both bounds once before selecting. Bounds
// may be computations; neither their result nor the value is recomputed during
// selection. NaN passes through unchanged rather than becoming a bound.
func NewBound(value, lower, upper core.Primitive) core.Primitive {
	return transport.NewPipe(
		store.NewRecord(
			transport.NewPipe(value, store.NewKey("value")),
			transport.NewPipe(lower, store.NewKey("lower")),
			transport.NewPipe(upper, store.NewKey("upper")),
		),
		logic.NewGate(
			NewLess[float64](store.NewGet("value"), store.NewGet("lower")),
			store.NewGet("lower"),
			logic.NewGate(NewGreater[float64](store.NewGet("value"), store.NewGet("upper")), store.NewGet("upper"), store.NewGet("value")),
		),
	)
}
