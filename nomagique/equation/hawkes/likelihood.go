package hawkes

import (
	"github.com/theapemachine/symm/nomagique/arithmetic"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

// NewLikelihood is the bivariate exponential Hawkes log likelihood, given
// natural parameters, events, origin and horizon. It preserves strict origin
// exclusion, equal-time grouping semantics and pre-origin kernel history.
// Invalid parameters are explicit errors. No stationary fit, optimization,
// significance calibration or parameter-bound selection is claimed here.
func NewLikelihood() core.Primitive {
	return logic.NewGate(
		equation.NewAll(
			equation.NewAll(
				transport.NewPipe(store.NewGet("mu_x"), logic.NewFinite()),
				equation.NewGreater[float64](store.NewGet("mu_x"), store.NewConstant(core.From(0.0))),
			),
			equation.NewAll(
				transport.NewPipe(store.NewGet("mu_y"), logic.NewFinite()),
				equation.NewGreater[float64](store.NewGet("mu_y"), store.NewConstant(core.From(0.0))),
			),
			equation.NewAll(
				transport.NewPipe(store.NewGet("beta"), logic.NewFinite()),
				equation.NewGreater[float64](store.NewGet("beta"), store.NewConstant(core.From(0.0))),
			),
			equation.NewAll(
				transport.NewPipe(store.NewGet("alpha_xx"), logic.NewFinite()),
				equation.NewLessEqual[float64](store.NewConstant(core.From(0.0)), store.NewGet("alpha_xx")),
			),
			equation.NewAll(
				transport.NewPipe(store.NewGet("alpha_xy"), logic.NewFinite()),
				equation.NewLessEqual[float64](store.NewConstant(core.From(0.0)), store.NewGet("alpha_xy")),
			),
			equation.NewAll(
				transport.NewPipe(store.NewGet("alpha_yx"), logic.NewFinite()),
				equation.NewLessEqual[float64](store.NewConstant(core.From(0.0)), store.NewGet("alpha_yx")),
			),
			equation.NewAll(
				transport.NewPipe(store.NewGet("alpha_yy"), logic.NewFinite()),
				equation.NewLessEqual[float64](store.NewConstant(core.From(0.0)), store.NewGet("alpha_yy")),
			),
			transport.NewPipe(store.NewGet("origin"), logic.NewFinite()),
			transport.NewPipe(store.NewGet("horizon"), logic.NewFinite()),
			equation.NewGreater[float64](store.NewGet("horizon"), store.NewGet("origin")),
			equation.NewGreater[float64](
				transport.NewPipe(store.NewGet("events"), transport.NewSpread[core.Primitive](), equation.NewCount()),
				store.NewConstant(core.From(0.0)),
			),
			transport.NewPipe(
				store.NewGet("events"),
				transport.NewSpread[core.Primitive](),
				transport.NewMap(
					equation.NewAll(
						transport.NewPipe(store.NewGet("at"), logic.NewFinite()),
						equation.NewAny(
							equation.NewEqual[float64](store.NewGet("side"), store.NewConstant(core.From(0.0))),
							equation.NewEqual[float64](store.NewGet("side"), store.NewConstant(core.From(1.0))),
						),
					),
				),
				logic.NewAnd(transport.NewIO(core.From(true))),
			),
		),
		transport.NewPipe(
			store.NewRecord(
				transport.NewPipe(),
				transport.NewPipe(equation.NewDifference[float64](store.NewGet("horizon"), store.NewGet("origin")), store.NewKey("span")),
				transport.NewPipe(NewEventLikelihood(), store.NewKey("scored")),
				transport.NewPipe(NewIntegralSupport(store.NewConstant(core.From(0.0))), store.NewKey("integral_x")),
				transport.NewPipe(NewIntegralSupport(store.NewConstant(core.From(1.0))), store.NewKey("integral_y")),
				transport.NewPipe(NewIntegralDerivative(store.NewConstant(core.From(0.0))), store.NewKey("integral_x_beta")),
				transport.NewPipe(NewIntegralDerivative(store.NewConstant(core.From(1.0))), store.NewKey("integral_y_beta")),
			),
			store.NewRecord(
				transport.NewPipe(),
				transport.NewPipe(
					transport.NewPipe(
						transport.NewPipe(store.NewGet("scored"), transport.NewSpread[core.Primitive](), transport.NewMap(store.NewGet("log_intensity"))),
						arithmetic.NewAdd[float64](transport.NewIO(core.From(0.0))),
					),
					store.NewKey("log_sum"),
				),
				transport.NewPipe(NewCompensator(), store.NewKey("compensator")),
			),
			store.NewRecord(
				transport.NewPipe(),
				transport.NewPipe(
					equation.NewDifference[float64](store.NewGet("log_sum"), store.NewGet("compensator")),
					store.NewKey("log_likelihood"),
				),
			),
		),
		logic.NewReject(core.ErrDomain),
	)
}
