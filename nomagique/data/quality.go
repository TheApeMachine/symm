package data

import (
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

// NewQuality derives fresh maturity/SNR facts from optional estimator fields.
// The source Finalize precedence is preserved, including requiring support>1
// before Mahalanobis overrides scalar SNR. This composition receives the facts
// record itself; external measurement serialization is a separate boundary.
func NewQuality() core.Primitive {
	initialize := store.NewRecord(
		transport.NewPipe(),
		transport.NewPipe(store.NewConstant(core.From(0.0)), store.NewKey("snr")),
		transport.NewPipe(store.NewConstant(core.From(false)), store.NewKey("snr_defined")),
		transport.NewPipe(
			equation.NewAny(store.NewHas("support"), store.NewHas("divergence"), store.NewHas("mahalanobis_snr")),
			store.NewKey("estimated"),
		),
	)
	scalar := logic.NewGate(
		equation.NewAll(store.NewHas("divergence"), store.NewHas("noise_variance")),
		logic.NewGate(
			equation.NewGreater[float64](store.NewGet("noise_variance"), store.NewConstant(core.From(0.0))),
			store.NewRecord(
				transport.NewPipe(),
				transport.NewPipe(
					equation.NewRatio[float64](
						transport.NewPipe(store.NewGet("divergence"), calculus.NewSquare(transport.NewIO(core.From(0.0)))),
						store.NewGet("noise_variance"),
					),
					store.NewKey("snr"),
				),
				transport.NewPipe(store.NewConstant(core.From(true)), store.NewKey("snr_defined")),
			),
			transport.NewPipe(),
		),
		transport.NewPipe(),
	)
	supported := transport.NewPipe(
		store.NewRecord(
			transport.NewPipe(),
			transport.NewPipe(
				equation.NewDifference[float64](
					store.NewConstant(core.From(1.0)),
					equation.NewRatio[float64](store.NewConstant(core.From(1.0)), store.NewGet("support")),
				),
				store.NewKey("maturity"),
			),
		),
		logic.NewGate(
			store.NewHas("mahalanobis_snr"),
			logic.NewGate(
				equation.NewLessEqual[float64](store.NewConstant(core.From(0.0)), store.NewGet("mahalanobis_snr")),
				store.NewRecord(
					transport.NewPipe(),
					transport.NewPipe(store.NewGet("mahalanobis_snr"), store.NewKey("snr")),
					transport.NewPipe(store.NewConstant(core.From(true)), store.NewKey("snr_defined")),
				),
				transport.NewPipe(),
			),
			transport.NewPipe(),
		),
	)
	maturity := logic.NewGate(
		store.NewHas("support"),
		logic.NewGate(
			equation.NewGreater[float64](store.NewGet("support"), store.NewConstant(core.From(1.0))),
			supported,
			store.NewRecord(transport.NewPipe(), transport.NewPipe(store.NewConstant(core.From(0.0)), store.NewKey("maturity"))),
		),
		store.NewRecord(transport.NewPipe(), transport.NewPipe(store.NewConstant(core.From(1.0)), store.NewKey("maturity"))),
	)
	finite := equation.NewAll(
		logic.NewGate(
			store.NewHas("support"),
			transport.NewPipe(store.NewGet("support"), logic.NewFinite()),
			store.NewConstant(core.From(true)),
		),
		logic.NewGate(
			store.NewHas("divergence"),
			transport.NewPipe(store.NewGet("divergence"), logic.NewFinite()),
			store.NewConstant(core.From(true)),
		),
		logic.NewGate(
			store.NewHas("noise_variance"),
			transport.NewPipe(store.NewGet("noise_variance"), logic.NewFinite()),
			store.NewConstant(core.From(true)),
		),
		logic.NewGate(
			store.NewHas("mahalanobis_snr"),
			transport.NewPipe(store.NewGet("mahalanobis_snr"), logic.NewFinite()),
			store.NewConstant(core.From(true)),
		),
	)
	return transport.NewMap(logic.NewGate(finite, transport.NewPipe(initialize, scalar, maturity), logic.NewReject(core.ErrDomain)))
}
