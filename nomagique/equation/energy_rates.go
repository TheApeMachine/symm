package equation

import (
	"github.com/theapemachine/symm/nomagique/arithmetic"
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

// NewEnergyRates composes r² / elapsed seconds over interval records.
func NewEnergyRates() core.Primitive {
	return transport.NewMap(
		NewRatio[float64](
			transport.NewPipe(store.NewGet("value"), calculus.NewSquare(transport.NewIO(core.From(0.0)))),
			transport.NewPipe(
				NewDifference[int64](store.NewGet("to"), store.NewGet("from")),
				calculus.NewConvert[int64, float64](),
				arithmetic.NewMultiply[float64](transport.NewIO(core.From(1e-9))),
			),
		),
	)
}
