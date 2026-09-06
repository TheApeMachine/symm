package hawkes

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

// NewLogGradient applies the source's chain rule for log(mu), log(beta), and
// log branching coordinates. Beta changes alpha=branch*beta as well as decay.
func NewLogGradient() core.Primitive {
	return transport.NewPipe(
		transport.NewFan(
			transport.NewPipe(),
			transport.NewIO(
				equation.NewProduct[float64](store.NewGet("d_mu_x"), store.NewGet("mu_x")),
				equation.NewProduct[float64](store.NewGet("d_mu_y"), store.NewGet("mu_y")),
				equation.NewSum[float64](
					equation.NewProduct[float64](store.NewGet("d_beta"), store.NewGet("beta")),
					equation.NewSum[float64](
						equation.NewSum[float64](
							equation.NewProduct[float64](store.NewGet("d_alpha_xx"), store.NewGet("alpha_xx")),
							equation.NewProduct[float64](store.NewGet("d_alpha_xy"), store.NewGet("alpha_xy")),
						),
						equation.NewSum[float64](
							equation.NewProduct[float64](store.NewGet("d_alpha_yx"), store.NewGet("alpha_yx")),
							equation.NewProduct[float64](store.NewGet("d_alpha_yy"), store.NewGet("alpha_yy")),
						),
					),
				),
				equation.NewProduct[float64](store.NewGet("d_alpha_xx"), store.NewGet("alpha_xx")),
				equation.NewProduct[float64](store.NewGet("d_alpha_xy"), store.NewGet("alpha_xy")),
				equation.NewProduct[float64](store.NewGet("d_alpha_yx"), store.NewGet("alpha_yx")),
				equation.NewProduct[float64](store.NewGet("d_alpha_yy"), store.NewGet("alpha_yy")),
			),
		),
		transport.NewCollect[float64](),
	)
}
