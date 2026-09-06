package equation

import (
	"github.com/theapemachine/symm/nomagique/arithmetic"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

// NewCount is addition over unit contributions. It counts delivered objects,
// regardless of their payload type. Collection members must first be Spread.
func NewCount() core.Primitive {
	return transport.NewMapReduce(store.NewConstant(core.From(1.0)), arithmetic.NewAdd[float64](transport.NewIO(core.From(0.0))))
}
