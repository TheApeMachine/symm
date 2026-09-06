package causal

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

// NewFeatures requires the treatment to be represented, and excludes the
// outcome from the predictors. Positional shape validation belongs to Gather/At.
func NewFeatures() core.Primitive {
	context := store.NewRetained(nil)
	return transport.NewPipe(
		context,
		equation.NewAll(
			transport.NewPipe(
				store.NewGet("features"),
				transport.NewSpread[float64](),
				transport.NewMap(equation.NewEqual[float64](
					transport.NewPipe(), transport.NewApply(store.NewGet("treatment"), context))),
				logic.NewOr(transport.NewIO(core.From(false))),
			),
			transport.NewPipe(
				store.NewGet("features"),
				transport.NewSpread[float64](),
				transport.NewMap(
					transport.NewPipe(
						equation.NewEqual[float64](
							transport.NewPipe(), transport.NewApply(store.NewGet("target"), context)),
						logic.NewNot(transport.NewIO(core.From(false))),
					),
				),
				logic.NewAnd(transport.NewIO(core.From(true))),
			),
		),
	)
}
