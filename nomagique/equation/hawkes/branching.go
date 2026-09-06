package hawkes

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/equation/linear"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

// NewBranching composes the source's offspring and stationary-mean formulas.
// alpha's column names identify the parent side. Supercritical parameters still
// report their radius/first generation; stationary means and total descendants
// are marked undefined rather than emitting non-stationary expectations.
func NewBranching() core.Primitive {
	return logic.NewGate(
		equation.NewAll(
			transport.NewPipe(store.NewGet("mu_x"), logic.NewFinite()),
			transport.NewPipe(store.NewGet("mu_y"), logic.NewFinite()),
			transport.NewPipe(store.NewGet("alpha_xx"), logic.NewFinite()),
			transport.NewPipe(store.NewGet("alpha_xy"), logic.NewFinite()),
			transport.NewPipe(store.NewGet("alpha_yx"), logic.NewFinite()),
			transport.NewPipe(store.NewGet("alpha_yy"), logic.NewFinite()),
			transport.NewPipe(store.NewGet("beta"), logic.NewFinite()),
			equation.NewLessEqual[float64](store.NewConstant(core.From(0.0)), store.NewGet("mu_x")),
			equation.NewLessEqual[float64](store.NewConstant(core.From(0.0)), store.NewGet("mu_y")),
			equation.NewLessEqual[float64](store.NewConstant(core.From(0.0)), store.NewGet("alpha_xx")),
			equation.NewLessEqual[float64](store.NewConstant(core.From(0.0)), store.NewGet("alpha_xy")),
			equation.NewLessEqual[float64](store.NewConstant(core.From(0.0)), store.NewGet("alpha_yx")),
			equation.NewLessEqual[float64](store.NewConstant(core.From(0.0)), store.NewGet("alpha_yy")),
			equation.NewGreater[float64](store.NewGet("beta"), store.NewConstant(core.From(0.0))),
		),
		transport.NewPipe(
			store.NewRecord(
				transport.NewPipe(),
				transport.NewPipe(equation.NewRatio[float64](store.NewGet("alpha_xx"), store.NewGet("beta")), store.NewKey("a")),
				transport.NewPipe(equation.NewRatio[float64](store.NewGet("alpha_xy"), store.NewGet("beta")), store.NewKey("b")),
				transport.NewPipe(equation.NewRatio[float64](store.NewGet("alpha_yx"), store.NewGet("beta")), store.NewKey("c")),
				transport.NewPipe(equation.NewRatio[float64](store.NewGet("alpha_yy"), store.NewGet("beta")), store.NewKey("d")),
			),
			store.NewRecord(
				transport.NewPipe(),
				transport.NewPipe(linear.NewSpectralRadius2(), store.NewKey("spectral_radius")),
				transport.NewPipe(
					equation.NewDifference[float64](
						equation.NewProduct[float64](
							equation.NewDifference[float64](store.NewConstant(core.From(1.0)), store.NewGet("a")),
							equation.NewDifference[float64](store.NewConstant(core.From(1.0)), store.NewGet("d")),
						),
						equation.NewProduct[float64](store.NewGet("b"), store.NewGet("c")),
					),
					store.NewKey("stationary_determinant"),
				),
				transport.NewPipe(equation.NewSum[float64](store.NewGet("a"), store.NewGet("c")), store.NewKey("offspring_x")),
				transport.NewPipe(equation.NewSum[float64](store.NewGet("b"), store.NewGet("d")), store.NewKey("offspring_y")),
			),
			store.NewRecord(
				transport.NewPipe(),
				transport.NewPipe(
					equation.NewLess[float64](store.NewGet("spectral_radius"), store.NewConstant(core.From(1.0))),
					store.NewKey("defined"),
				),
			),
			logic.NewGate(
				store.NewGet("defined"),
				store.NewRecord(
					transport.NewPipe(),
					transport.NewPipe(
						equation.NewRatio[float64](
							equation.NewSum[float64](
								equation.NewProduct[float64](
									equation.NewDifference[float64](store.NewConstant(core.From(1.0)), store.NewGet("d")),
									store.NewGet("mu_x"),
								),
								equation.NewProduct[float64](store.NewGet("b"), store.NewGet("mu_y")),
							),
							store.NewGet("stationary_determinant"),
						),
						store.NewKey("mean_x"),
					),
					transport.NewPipe(
						equation.NewRatio[float64](
							equation.NewSum[float64](
								equation.NewProduct[float64](store.NewGet("c"), store.NewGet("mu_x")),
								equation.NewProduct[float64](
									equation.NewDifference[float64](store.NewConstant(core.From(1.0)), store.NewGet("a")),
									store.NewGet("mu_y"),
								),
							),
							store.NewGet("stationary_determinant"),
						),
						store.NewKey("mean_y"),
					),
					transport.NewPipe(
						equation.NewDifference[float64](
							equation.NewRatio[float64](
								equation.NewSum[float64](equation.NewDifference[float64](store.NewConstant(core.From(1.0)), store.NewGet("d")), store.NewGet("c")),
								store.NewGet("stationary_determinant"),
							),
							store.NewConstant(core.From(1.0)),
						),
						store.NewKey("descendants_x"),
					),
					transport.NewPipe(
						equation.NewDifference[float64](
							equation.NewRatio[float64](
								equation.NewSum[float64](store.NewGet("b"), equation.NewDifference[float64](store.NewConstant(core.From(1.0)), store.NewGet("a"))),
								store.NewGet("stationary_determinant"),
							),
							store.NewConstant(core.From(1.0)),
						),
						store.NewKey("descendants_y"),
					),
				),
				store.NewRecord(
					transport.NewPipe(),
					transport.NewPipe(
						equation.NewRatio[float64](store.NewConstant(core.From(0.0)), store.NewConstant(core.From(0.0))),
						store.NewKey("mean_x"),
					),
					transport.NewPipe(
						equation.NewRatio[float64](store.NewConstant(core.From(0.0)), store.NewConstant(core.From(0.0))),
						store.NewKey("mean_y"),
					),
					transport.NewPipe(
						equation.NewRatio[float64](store.NewConstant(core.From(0.0)), store.NewConstant(core.From(0.0))),
						store.NewKey("descendants_x"),
					),
					transport.NewPipe(
						equation.NewRatio[float64](store.NewConstant(core.From(0.0)), store.NewConstant(core.From(0.0))),
						store.NewKey("descendants_y"),
					),
				),
			),
		),
		logic.NewReject(core.ErrDomain),
	)
}
