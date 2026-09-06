package equation

import (
	"github.com/theapemachine/symm/nomagique/arithmetic"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

// NewCumulativeSum is Add with explicit feedback through retention. The Add
// itself remains unchanged and can still be used as a per-run reduction.
func NewCumulativeSum() core.Primitive {
	retained := store.NewRetained(core.From(0.0))
	return transport.NewMap(transport.NewPipe(arithmetic.NewAdd[float64](retained), retained))
}
