package equation

import (
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

// NewCausalResidual measures against the moments that existed before the
// observation. Its configured estimator owns recurrence and any forgetting.
// The source's zero-dispersion convention is explicit: use |residual| as the
// score scale, or zero when the residual is zero. Both the prior variance and
// the actual score scale are exposed, rather than conflated.
func NewCausalResidual(moments core.Primitive) core.Primitive {
	return transport.NewPipe(
		moments,
		transport.NewMap(
			transport.NewPipe(
				store.NewRecord(
					transport.NewPipe(),
					transport.NewPipe(NewGreater[float64](store.NewGet("prior_count"), store.NewConstant(core.From(0.0))), store.NewKey("has_prior")),
					transport.NewPipe(
						logic.NewGate(
							NewGreater[float64](store.NewGet("prior_count"), store.NewConstant(core.From(0.0))),
							store.NewGet("prior_mean"),
							store.NewGet("value"),
						),
						store.NewKey("baseline"),
					),
					transport.NewPipe(
						logic.NewGate(
							NewGreater[float64](store.NewGet("prior_count"), store.NewConstant(core.From(1.0))),
							NewRatio[float64](
								store.NewGet("prior_m2"),
								NewDifference[float64](store.NewGet("prior_count"), store.NewConstant(core.From(1.0))),
							),
							store.NewConstant(core.From(0.0)),
						),
						store.NewKey("prior_variance"),
					),
					transport.NewPipe(
						NewDifference[float64](
							store.NewConstant(core.From(1.0)),
							NewRatio[float64](
								store.NewConstant(core.From(1.0)),
								NewSum[float64](store.NewGet("prior_count"), store.NewConstant(core.From(1.0))),
							),
						),
						store.NewKey("maturity"),
					),
				),
				store.NewRecord(
					transport.NewPipe(),
					transport.NewPipe(
						NewDifference[float64](store.NewGet("value"), store.NewGet("baseline")), store.NewKey("residual")),
				),
				store.NewRecord(
					transport.NewPipe(),
					transport.NewPipe(
						logic.NewGate(
							NewGreater[float64](store.NewGet("prior_variance"), store.NewConstant(core.From(0.0))),
							transport.NewPipe(store.NewGet("prior_variance"), calculus.NewSqrt(transport.NewIO(core.From(0.0)))),
							transport.NewPipe(store.NewGet("residual"), calculus.NewAbsolute(transport.NewIO(core.From(0.0)))),
						),
						store.NewKey("score_scale"),
					),
				),
				store.NewRecord(
					transport.NewPipe(),
					transport.NewPipe(
						logic.NewGate(
							NewGreater[float64](store.NewGet("score_scale"), store.NewConstant(core.From(0.0))),
							NewRatio[float64](store.NewGet("residual"), store.NewGet("score_scale")),
							store.NewConstant(core.From(0.0)),
						),
						store.NewKey("zscore"),
					),
					transport.NewPipe(store.NewGet("variance"), store.NewKey("noise_variance")),
				),
			),
		),
	)
}
