package hawkes

import (
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

// NewCompensatorDerivative includes the derivative of both the kernel
// numerator and the 1/beta scale. Omitting either changes the fitted model.
func NewCompensatorDerivative() core.Primitive {
	return equation.NewSum[float64](
		equation.NewProduct[float64](
			equation.NewSum[float64](store.NewGet("alpha_xx"), store.NewGet("alpha_yx")),
			equation.NewDifference[float64](
				equation.NewRatio[float64](store.NewGet("integral_x_beta"), store.NewGet("beta")),
				equation.NewRatio[float64](
					store.NewGet("integral_x"),
					transport.NewPipe(store.NewGet("beta"), calculus.NewSquare(transport.NewIO(core.From(0.0)))),
				),
			),
		),
		equation.NewProduct[float64](
			equation.NewSum[float64](store.NewGet("alpha_xy"), store.NewGet("alpha_yy")),
			equation.NewDifference[float64](
				equation.NewRatio[float64](store.NewGet("integral_y_beta"), store.NewGet("beta")),
				equation.NewRatio[float64](
					store.NewGet("integral_y"),
					transport.NewPipe(store.NewGet("beta"), calculus.NewSquare(transport.NewIO(core.From(0.0)))),
				),
			),
		),
	)
}
