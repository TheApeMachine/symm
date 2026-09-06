package equation

import (
	"github.com/theapemachine/symm/nomagique/arithmetic"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

// NewWeightedMean consumes KV-shaped records with "weight" and "value" keys.
func NewWeightedMean() core.Primitive {
	return NewRatio[float64](
		transport.NewMapReduce(
			NewProduct[float64](store.NewGet("weight"), store.NewGet("value")),
			arithmetic.NewAdd[float64](transport.NewIO(core.From(0.0))),
		),
		transport.NewMapReduce(store.NewGet("weight"), arithmetic.NewAdd[float64](transport.NewIO(core.From(0.0)))),
	)
}
