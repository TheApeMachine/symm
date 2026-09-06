package equation

import (
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

// NewLogMomentView reads a log-space Welford record against prior moments.
// It does not log the input again. Noise and zscore follow the original joint
// estimator's minimum prior support and positive-dispersion requirements.
func NewLogMomentView() core.Primitive {
	return transport.NewPipe(
		store.NewRecord(
			transport.NewPipe(),
			transport.NewPipe(NewDifference[float64](store.NewGet("value"), store.NewGet("prior_mean")), store.NewKey("residual")),
			transport.NewPipe(store.NewGet("prior_mean"), calculus.NewExp(transport.NewIO(core.From(0.0))), store.NewKey("baseline")),
			transport.NewPipe(NewGreater[float64](store.NewGet("prior_count"), store.NewConstant(core.From(0.0))), store.NewKey("has_prior")),
			transport.NewPipe(
				NewAll(
					NewGreater[float64](store.NewGet("prior_count"), store.NewConstant(core.From(1.0))),
					NewGreater[float64](store.NewGet("prior_m2"), store.NewConstant(core.From(0.0))),
				),
				store.NewKey("noise_defined"),
			),
		),
		store.NewRecord(
			transport.NewPipe(),
			transport.NewPipe(store.NewGet("residual"), calculus.NewExp(transport.NewIO(core.From(0.0))), store.NewKey("ratio")),
			transport.NewPipe(
				logic.NewGate(
					store.NewGet("noise_defined"),
					transport.NewPipe(
						NewRatio[float64](
							store.NewGet("prior_m2"),
							NewDifference[float64](store.NewGet("prior_count"), store.NewConstant(core.From(1.0))),
						),
						calculus.NewSqrt(transport.NewIO(core.From(0.0))),
					),
					store.NewConstant(core.From(0.0)),
				),
				store.NewKey("noise"),
			),
		),
		store.NewRecord(
			transport.NewPipe(),
			transport.NewPipe(
				logic.NewGate(
					store.NewGet("noise_defined"),
					NewRatio[float64](store.NewGet("residual"), store.NewGet("noise")),
					store.NewConstant(core.From(0.0)),
				),
				store.NewKey("zscore"),
			),
		),
	)
}
