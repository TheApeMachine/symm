package rls

import (
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/matrix"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
	"github.com/theapemachine/symm/nomagique/vector"
)

// NewPrediction evaluates the supplied square-root coefficient posterior.
// observationCount supplies independent observation-noise multiplicity; use 1
// for one forecast and N for a sum whose design vector is already aggregated.
// Prediction is evaluated before update when placed before NewUpdate.
func NewPrediction(observationCount core.Primitive) core.Primitive {
	return transport.NewPipe(
		store.NewRecord(
			transport.NewPipe(),
			transport.NewPipe(
				vector.NewDot(
					transport.NewPipe(store.NewGet("beta"), transport.NewSpread[float64]()),
					transport.NewPipe(store.NewGet("design"), transport.NewSpread[float64]()),
				),
				store.NewKey("prediction"),
			),
			transport.NewPipe(
				matrix.NewVector(transport.NewPipe(store.NewGet("root"), matrix.NewTranspose[float64]()), store.NewGet("design")),
				store.NewKey("factor"),
			),
			transport.NewPipe(store.NewConstant(core.From(0.0)), store.NewKey("scale")),
			transport.NewPipe(store.NewConstant(core.From(0.0)), store.NewKey("degrees_of_freedom")),
			transport.NewPipe(store.NewConstant(core.From(false)), store.NewKey("ready")),
		),
		logic.NewGate(
			equation.NewAll(
				equation.NewGreater[float64](store.NewGet("noise_shape"), store.NewConstant(core.From(0.0))),
				equation.NewGreater[float64](store.NewGet("noise_scale"), store.NewConstant(core.From(0.0))),
			),
			transport.NewPipe(
				store.NewRecord(
					transport.NewPipe(),
					transport.NewPipe(
						equation.NewProduct[float64](
							equation.NewRatio[float64](store.NewGet("noise_scale"), store.NewGet("noise_shape")),
							equation.NewSum[float64](
								observationCount,
								transport.NewPipe(store.NewGet("factor"), transport.NewSpread[float64](), equation.NewEnergy()),
							),
						),
						store.NewKey("predictive_variance"),
					),
				),
				logic.NewGate(
					equation.NewAll(
						equation.NewGreater[float64](store.NewGet("predictive_variance"), store.NewConstant(core.From(0.0))),
						transport.NewPipe(store.NewGet("predictive_variance"), logic.NewFinite()),
					),
					store.NewRecord(
						transport.NewPipe(),
						transport.NewPipe(store.NewGet("predictive_variance"), calculus.NewSqrt(transport.NewIO(core.From(0.0))), store.NewKey("scale")),
						transport.NewPipe(
							equation.NewProduct[float64](store.NewConstant(core.From(2.0)), store.NewGet("noise_shape")),
							store.NewKey("degrees_of_freedom"),
						),
						transport.NewPipe(store.NewConstant(core.From(true)), store.NewKey("ready")),
					),
					logic.NewReject(core.ErrDomain),
				),
			),
			transport.NewPipe(),
		),
	)
}
