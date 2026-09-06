package equation

import (
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

// NewRetentionFactor evaluates (1-1/memory)^gap for memory > 1; otherwise
// retention is one, as in the supplied prior. The positive-base power is already
// expressible by Log, Multiply and Exp, so there is no new Power kernel.
func NewRetentionFactor() core.Primitive {
	return logic.NewGate(
		NewGreater[float64](store.NewGet("memory"), store.NewConstant(core.From(1.0))),
		transport.NewPipe(
			NewProduct[float64](
				store.NewGet("gap"),
				transport.NewPipe(
					NewDifference[float64](
						store.NewConstant(core.From(1.0)),
						NewRatio[float64](store.NewConstant(core.From(1.0)), store.NewGet("memory")),
					),
					calculus.NewLog(transport.NewIO(core.From(0.0))),
				),
			),
			calculus.NewExp(transport.NewIO(core.From(0.0))),
		),
		store.NewConstant(core.From(1.0)),
	)
}
