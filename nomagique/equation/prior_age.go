package equation

import (
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

// NewPriorAge ages total weight only. Normalized moment/support do not decay
// under a uniform change of weight, avoiding squared-weight underflow.
// Epochs are uint64; subtraction occurs only after the ordering gate.
func NewPriorAge() core.Primitive {
	return logic.NewGate(
		NewGreater[uint64](store.NewGet("epoch"), store.NewGet("last_epoch")),
		transport.NewPipe(
			store.NewRecord(
				transport.NewPipe(),
				transport.NewPipe(
					NewDifference[uint64](store.NewGet("epoch"), store.NewGet("last_epoch")),
					calculus.NewConvert[uint64, float64](),
					store.NewKey("gap"),
				),
			),
			store.NewRecord(
				transport.NewPipe(),
				transport.NewPipe(
					NewProduct[float64](store.NewGet("weight"), NewRetentionFactor()), store.NewKey("weight")),
			),
			store.NewRecord(transport.NewPipe(), transport.NewPipe(store.NewGet("epoch"), store.NewKey("last_epoch"))),
		),
		transport.NewPipe(),
	)
}
