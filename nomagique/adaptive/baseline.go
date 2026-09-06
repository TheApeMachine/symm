package adaptive

import (
	"github.com/theapemachine/symm/nomagique/algo"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

// NewBaseline scores against prior moments, then applies the configured
// observation-driven window's shedding factor to its retained moments.
// A different window is substituted by configuration, not an estimator mode.
func NewBaseline(window core.Primitive) core.Primitive {
	return equation.NewCausalResidual(
		algo.NewWelford(
			transport.NewPipe(
				store.NewRecord(
					transport.NewPipe(),
					transport.NewPipe(store.NewGet("value"), window,
						store.NewGet("shed_ratio"), store.NewKey("retain")),
				),
				equation.NewShedMoments(),
			),
		),
	)
}
