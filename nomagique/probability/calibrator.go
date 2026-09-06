package probability

import (
	"github.com/theapemachine/symm/nomagique/arithmetic"
	"github.com/theapemachine/symm/nomagique/collection"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

// NewCalibrator scores against retained prior errors before appending a sample.
// Retention is a configured collection transform: identity for all history,
// Tail for a bounded history, or another composed selection policy. Admission
// gates the entire state transition: an empty rejected run must not query the
// retention owner and replay the preceding sample.
func NewCalibrator(retention core.Primitive) core.Primitive {
	history := store.NewRetained(core.From([]float64{}))
	sample := store.NewRetained(core.From(0.0))
	prior := transport.NewApply(transport.NewPipe(
		transport.NewSpread[float64]()), history,
	)
	score := logic.NewGate(
		transport.NewPipe(
			transport.NewFan(
				transport.NewPipe(),
				transport.NewIO(
					transport.NewApply(transport.NewPipe(
						transport.NewSpread[float64](),
						equation.NewCount(),
					), history),
					store.NewConstant(core.From(0.0)),
				),
			),
			transport.NewCollect[float64](),
			logic.NewGreater[float64](),
		),
		store.NewRecord(
			transport.NewPipe(
				equation.NewRatio[float64](
					transport.NewPipe(
						prior,
						transport.NewMap(
							logic.NewGate(
								transport.NewPipe(
									transport.NewFan(
										transport.NewPipe(),
										transport.NewIO(
											transport.NewPipe(),
											transport.NewApply(sample, nil),
										),
									),
									transport.NewCollect[float64](),
									logic.NewGreater[float64](),
								),
								store.NewConstant(core.From(1.0)),
								store.NewConstant(core.From(0.0)),
							),
						),
						arithmetic.NewAdd[float64](transport.NewIO(core.From(0.0))),
					),
					transport.NewApply(transport.NewPipe(
						transport.NewSpread[float64](), equation.NewCount(),
					), history),
				),
				store.NewKey("value"),
			),
			transport.NewPipe(store.NewConstant(core.From(true)), store.NewKey("ready")),
			transport.NewPipe(
				transport.NewApply(transport.NewPipe(
					transport.NewSpread[float64](), equation.NewCount(),
				), history),
				store.NewKey("prior_count"),
			),
		),
		store.NewRecord(
			transport.NewPipe(store.NewConstant(core.From(0.0)), store.NewKey("value")),
			transport.NewPipe(store.NewConstant(core.From(false)), store.NewKey("ready")),
			transport.NewPipe(store.NewConstant(core.From(0.0)), store.NewKey("prior_count")),
		),
	)
	return transport.NewMap(
		logic.NewGate(
			logic.NewFinite(),
			transport.NewPipe(
				sample,
				transport.NewFan(
					transport.NewPipe(),
					transport.NewIO(score, transport.NewPipe(
						collection.NewAppend[float64](history),
						retention, history, transport.NewDiscard(),
					)),
				),
			),
			logic.NewReject(core.ErrShape),
		),
	)
}
