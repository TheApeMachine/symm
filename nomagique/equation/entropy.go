package equation

import (
	"github.com/theapemachine/symm/nomagique/arithmetic"
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

// NewEntropy composes -sum(p*log(p)). The zero-mass contribution is its limiting
// value zero; negative inputs retain the logarithm's undefined-domain result.
func NewEntropy() core.Primitive {
	return transport.NewMapReduce(
		logic.NewGate(
			transport.NewPipe(
				transport.NewFan(transport.NewPipe(), transport.NewIO(transport.NewPipe(), store.NewConstant(core.From(0.0)))),
				transport.NewCollect[float64](),
				logic.NewEqual[float64](),
			),
			store.NewConstant(core.From(0.0)),
			transport.NewPipe(
				NewProduct[float64](transport.NewPipe(), calculus.NewLog(transport.NewIO(core.From(0.0)))),
				calculus.NewNegate(transport.NewIO(core.From(0.0))),
			),
		),
		arithmetic.NewAdd[float64](transport.NewIO(core.From(0.0))),
	)
}
