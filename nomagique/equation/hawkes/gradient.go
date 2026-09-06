package hawkes

import (
	"github.com/theapemachine/symm/nomagique/arithmetic"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

// NewGradient adds natural-parameter derivatives to a NewLikelihood result.
// The output gradient order is mu_x,mu_y,alpha_xx,alpha_xy,alpha_yx,alpha_yy,beta.
func NewGradient() core.Primitive {
	return transport.NewPipe(
		store.NewRecord(
			transport.NewPipe(),
			transport.NewPipe(
				equation.NewDifference[float64](
					NewScore(
						store.NewConstant(core.From(0.0)),
						equation.NewRatio[float64](store.NewConstant(core.From(1.0)), store.NewGet("intensity")),
					),
					store.NewGet("span"),
				),
				store.NewKey("d_mu_x"),
			),
			transport.NewPipe(
				equation.NewDifference[float64](
					NewScore(
						store.NewConstant(core.From(1.0)),
						equation.NewRatio[float64](store.NewConstant(core.From(1.0)), store.NewGet("intensity")),
					),
					store.NewGet("span"),
				),
				store.NewKey("d_mu_y"),
			),
			transport.NewPipe(
				equation.NewDifference[float64](
					NewScore(store.NewConstant(core.From(0.0)), equation.NewRatio[float64](store.NewGet("support_x"), store.NewGet("intensity"))),
					equation.NewRatio[float64](store.NewGet("integral_x"), store.NewGet("beta")),
				),
				store.NewKey("d_alpha_xx"),
			),
			transport.NewPipe(
				equation.NewDifference[float64](
					NewScore(store.NewConstant(core.From(0.0)), equation.NewRatio[float64](store.NewGet("support_y"), store.NewGet("intensity"))),
					equation.NewRatio[float64](store.NewGet("integral_y"), store.NewGet("beta")),
				),
				store.NewKey("d_alpha_xy"),
			),
			transport.NewPipe(
				equation.NewDifference[float64](
					NewScore(store.NewConstant(core.From(1.0)), equation.NewRatio[float64](store.NewGet("support_x"), store.NewGet("intensity"))),
					equation.NewRatio[float64](store.NewGet("integral_x"), store.NewGet("beta")),
				),
				store.NewKey("d_alpha_yx"),
			),
			transport.NewPipe(
				equation.NewDifference[float64](
					NewScore(store.NewConstant(core.From(1.0)), equation.NewRatio[float64](store.NewGet("support_y"), store.NewGet("intensity"))),
					equation.NewRatio[float64](store.NewGet("integral_y"), store.NewGet("beta")),
				),
				store.NewKey("d_alpha_yy"),
			),
			transport.NewPipe(
				equation.NewDifference[float64](
					transport.NewPipe(
						transport.NewPipe(
							store.NewGet("scored"),
							transport.NewSpread[core.Primitive](),
							transport.NewMap(equation.NewRatio[float64](store.NewGet("intensity_beta"), store.NewGet("intensity"))),
						),
						arithmetic.NewAdd[float64](transport.NewIO(core.From(0.0))),
					),
					NewCompensatorDerivative(),
				),
				store.NewKey("d_beta"),
			),
		),
		store.NewRecord(
			transport.NewPipe(),
			transport.NewPipe(
				transport.NewPipe(
					transport.NewFan(
						transport.NewPipe(),
						transport.NewIO(
							store.NewGet("d_mu_x"),
							store.NewGet("d_mu_y"),
							store.NewGet("d_alpha_xx"),
							store.NewGet("d_alpha_xy"),
							store.NewGet("d_alpha_yx"),
							store.NewGet("d_alpha_yy"),
							store.NewGet("d_beta"),
						),
					),
					transport.NewCollect[float64](),
				),
				store.NewKey("gradient"),
			),
		),
	)
}
