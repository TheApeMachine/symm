package learning

import (
	"github.com/theapemachine/symm/nomagique/algo"
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/matrix"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

// NewRLS supplies an affine intercept and the source's zero-mean coefficient
// prior with diagonal initial variance. Configuration is Primitive-valued;
// no constructor evaluates it. Features are []float64, target is optional.
// The output includes the prior predictive facts and current posterior state.
func NewRLS(dimension, variance, forgetting core.Primitive) core.Primitive {
	initialContext := store.NewRetained(nil)
	initial := transport.NewPipe(
		store.NewRecord(
			transport.NewPipe(equation.NewSum[float64](store.NewGet("dimension"), store.NewConstant(core.From(1.0))), store.NewKey("size")),
			transport.NewPipe(variance, store.NewKey("initial_variance")),
		),
		initialContext,
		logic.NewGate(
			equation.NewAll(
				equation.NewGreater[float64](store.NewGet("initial_variance"), store.NewConstant(core.From(0.0))),
				transport.NewPipe(store.NewGet("initial_variance"), logic.NewFinite()),
			),
			store.NewRecord(
				transport.NewPipe(
					transport.NewRange(transport.NewApply(store.NewGet("size"), initialContext)),
					transport.NewMap(store.NewConstant(core.From(0.0))),
					transport.NewCollect[float64](),
					store.NewKey("beta"),
				),
				transport.NewPipe(
					matrix.NewScale(
						transport.NewPipe(store.NewGet("size"), matrix.NewIdentity()),
						transport.NewPipe(store.NewGet("initial_variance"), calculus.NewSqrt(transport.NewIO(core.From(0.0)))),
					),
					store.NewKey("root"),
				),
				transport.NewPipe(store.NewConstant(core.From(0.0)), store.NewKey("noise_shape")),
				transport.NewPipe(store.NewConstant(core.From(0.0)), store.NewKey("noise_scale")),
			),
			logic.NewReject(core.ErrDomain),
		),
	)
	return transport.NewMap(
		transport.NewPipe(
			store.NewRecord(
				transport.NewPipe(),
				transport.NewPipe(dimension, store.NewKey("dimension")),
				transport.NewPipe(store.NewGet("features"), transport.NewSpread[float64](), equation.NewCount(), store.NewKey("feature_count")),
				transport.NewPipe(
					store.NewGet("features"),
					transport.NewFan(transport.NewPipe(), transport.NewIO(store.NewConstant(core.From(1.0)), transport.NewSpread[float64]())),
					transport.NewCollect[float64](),
					store.NewKey("design"),
				),
			),
			logic.NewGate(
				equation.NewAll(
					equation.NewGreater[float64](store.NewGet("dimension"), store.NewConstant(core.From(0.0))),
					equation.NewEqual[float64](store.NewGet("dimension"), store.NewGet("feature_count")),
				),
				algo.NewSquareRootRLS(initial, forgetting),
				logic.NewReject(core.ErrShape),
			),
		),
	)
}
