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

// NewUpdate composes the source's symmetric square-root rank-one update.
// Input retains prior beta/root/factor/prediction and target/lambda. It returns
// fresh beta/root and noise moments; it owns no recurrence or reset policy.
func NewUpdate() core.Primitive {
	prepare := store.NewRecord(
		transport.NewPipe(),
		transport.NewPipe(
			equation.NewSum[float64](
				store.NewGet("lambda"),
				transport.NewPipe(store.NewGet("factor"), transport.NewSpread[float64](), equation.NewEnergy()),
			),
			store.NewKey("alpha"),
		),
		transport.NewPipe(equation.NewDifference[float64](store.NewGet("target"), store.NewGet("prediction")), store.NewKey("innovation")),
		transport.NewPipe(store.NewGet("lambda"), calculus.NewSqrt(transport.NewIO(core.From(0.0))), store.NewKey("root_lambda")),
	)
	gain := store.NewRecord(
		transport.NewPipe(),
		transport.NewPipe(
			vector.NewScale(
				matrix.NewVector(store.NewGet("root"), store.NewGet("factor")),
				equation.NewRatio[float64](store.NewConstant(core.From(1.0)), store.NewGet("alpha")),
			),
			store.NewKey("gain"),
		),
		transport.NewPipe(
			equation.NewSum[float64](
				store.NewGet("alpha"),
				equation.NewProduct[float64](
					store.NewGet("root_lambda"),
					transport.NewPipe(store.NewGet("alpha"), calculus.NewSqrt(transport.NewIO(core.From(0.0)))),
				),
			),
			store.NewKey("gamma_denominator"),
		),
	)
	updated := store.NewRecord(
		transport.NewPipe(),
		transport.NewPipe(
			vector.NewSum(
				transport.NewPipe(store.NewGet("beta"), transport.NewSpread[float64]()),
				transport.NewPipe(vector.NewScale(store.NewGet("gain"), store.NewGet("innovation")), transport.NewSpread[float64]()),
			),
			store.NewKey("beta"),
		),
		transport.NewPipe(
			matrix.NewScale(
				matrix.NewDifference(
					store.NewGet("root"),
					matrix.NewOuter(
						vector.NewScale(store.NewGet("gain"), equation.NewRatio[float64](store.NewGet("alpha"), store.NewGet("gamma_denominator"))),
						store.NewGet("factor"),
					),
				),
				equation.NewRatio[float64](store.NewConstant(core.From(1.0)), store.NewGet("root_lambda")),
			),
			store.NewKey("root"),
		),
		transport.NewPipe(
			equation.NewSum[float64](
				equation.NewProduct[float64](store.NewGet("lambda"), store.NewGet("noise_shape")),
				store.NewConstant(core.From(0.5)),
			),
			store.NewKey("noise_shape"),
		),
		transport.NewPipe(
			equation.NewSum[float64](
				equation.NewProduct[float64](store.NewGet("lambda"), store.NewGet("noise_scale")),
				equation.NewRatio[float64](
					equation.NewProduct[float64](
						store.NewConstant(core.From(0.5)),
						transport.NewPipe(store.NewGet("innovation"), calculus.NewSquare(transport.NewIO(core.From(0.0)))),
					),
					store.NewGet("alpha"),
				),
			),
			store.NewKey("noise_scale"),
		),
	)
	return transport.NewPipe(
		prepare,
		logic.NewGate(
			equation.NewAll(
				equation.NewGreater[float64](store.NewGet("alpha"), store.NewConstant(core.From(0.0))),
				transport.NewPipe(store.NewGet("alpha"), logic.NewFinite()),
				transport.NewPipe(store.NewGet("innovation"), logic.NewFinite()),
			),
			transport.NewPipe(
				gain,
				updated,
				logic.NewGate(
					equation.NewAll(
						transport.NewPipe(store.NewGet("beta"), transport.NewSpread[float64](), vector.NewFinite()),
						transport.NewPipe(store.NewGet("root"), matrix.NewFinite()),
						transport.NewPipe(store.NewGet("noise_scale"), logic.NewFinite()),
					),
					transport.NewPipe(),
					logic.NewReject(core.ErrDomain),
				),
			),
			logic.NewReject(core.ErrDomain),
		),
	)
}
