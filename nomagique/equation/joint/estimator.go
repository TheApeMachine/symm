package joint

import (
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
	"github.com/theapemachine/symm/nomagique/vector"
)

// NewEstimator applies one configured moment estimator per coordinate.
// Input is a values []float64 record, already in log space. Other facts such as
// at remain in the output. Estimators is a source of actual Primitive endpoints;
// their count, rather than a hard-coded vector width, establishes the shape.
// The joint SNR is the source's diagonal standardized-energy mean, not a full
// inverse-covariance Mahalanobis estimate. Each endpoint owns its own history.
func NewEstimator(estimators core.Primitive) core.Primitive {
	channels := transport.NewPipe(
		store.NewGet("values"),
		transport.NewSpread[float64](),
		vector.NewApply(estimators),
		transport.NewMap(equation.NewLogMomentView()),
		transport.NewCollect[core.Primitive](),
	)
	energies := transport.NewPipe(
		store.NewGet("channels"),
		transport.NewSpread[core.Primitive](),
		transport.NewMap(
			logic.NewGate(
				store.NewGet("noise_defined"),
				transport.NewPipe(store.NewGet("zscore"), calculus.NewSquare(transport.NewIO(core.From(0.0)))),
				transport.NewDiscard(),
			),
		),
		transport.NewCollect[float64](),
	)
	return transport.NewMap(
		logic.NewGate(
			transport.NewPipe(store.NewGet("values"), transport.NewSpread[float64](), vector.NewFinite()),
			transport.NewPipe(
				store.NewRecord(transport.NewPipe(), transport.NewPipe(channels, store.NewKey("channels"))),
				store.NewRecord(transport.NewPipe(), transport.NewPipe(energies, store.NewKey("energies"))),
				store.NewRecord(
					transport.NewPipe(),
					transport.NewPipe(
						equation.NewGreater[float64](
							transport.NewPipe(store.NewGet("energies"), transport.NewSpread[float64](), equation.NewCount()),
							store.NewConstant(core.From(0.0)),
						),
						store.NewKey("snr_defined"),
					),
				),
				store.NewRecord(
					transport.NewPipe(),
					transport.NewPipe(
						logic.NewGate(
							store.NewGet("snr_defined"),
							transport.NewPipe(store.NewGet("energies"), transport.NewSpread[float64](), equation.NewMean()),
							store.NewConstant(core.From(0.0)),
						),
						store.NewKey("snr"),
					),
				),
			),
			logic.NewReject(core.ErrDomain),
		),
	)
}
