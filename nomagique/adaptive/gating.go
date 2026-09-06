package adaptive

import (
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

// NewGating suppresses values inside a configured threshold of inclusive
// moments. Estimation, threshold calculation and Boolean routing stay separate.
func NewGating(moments, threshold core.Primitive) core.Primitive {
	return transport.NewPipe(
		moments,
		transport.NewMap(
			logic.NewGate(
				equation.NewAll(
					equation.NewLess[float64](
						transport.NewPipe(
							equation.NewDifference[float64](store.NewGet("value"), store.NewGet("mean")),
							calculus.NewAbsolute(transport.NewIO(core.From(0.0))),
						),
						threshold,
					),
					equation.NewGreater[float64](store.NewGet("dispersion"), store.NewConstant(core.From(0.0))),
				),
				store.NewConstant(core.From(0.0)),
				store.NewGet("value"),
			),
		),
	)
}
