package equation

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

// NewStandardizer preserves the source's inclusive score, using moments after
// incorporation. It is deliberately distinct from NewCausalResidual.
func NewStandardizer(moments core.Primitive) core.Primitive {
	return transport.NewPipe(
		moments,
		transport.NewMap(
			store.NewRecord(
				transport.NewPipe(),
				transport.NewPipe(
					logic.NewGate(
						NewGreater[float64](store.NewGet("dispersion"), store.NewConstant(core.From(0.0))),
						NewRatio[float64](NewDifference[float64](store.NewGet("value"), store.NewGet("mean")), store.NewGet("dispersion")),
						store.NewConstant(core.From(0.0)),
					),
					store.NewKey("zscore"),
				),
			),
		),
	)
}
