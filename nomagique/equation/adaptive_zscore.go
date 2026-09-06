package equation

import (
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

// NewAdaptiveZScore uses log-space moments. The configured estimator determines
// whether its horizon is cumulative or adaptive. Non-positive logarithms keep
// the underlying real-domain result; they are not replaced by log(1).
func NewAdaptiveZScore(moments core.Primitive) core.Primitive {
	return transport.NewPipe(
		transport.NewMap(calculus.NewLog(transport.NewIO(core.From(0.0)))),
		NewCausalResidual(moments),
		transport.NewMap(
			store.NewRecord(
				transport.NewPipe(),
				transport.NewPipe(store.NewGet("baseline"), calculus.NewExp(transport.NewIO(core.From(0.0))), store.NewKey("baseline")),
				transport.NewPipe(store.NewGet("residual"), calculus.NewExp(transport.NewIO(core.From(0.0))), store.NewKey("ratio")),
				transport.NewPipe(store.NewGet("residual"), store.NewKey("divergence")),
			),
		),
	)
}
