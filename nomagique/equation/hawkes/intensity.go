package hawkes

import (
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

// NewIntensity evaluates one side's pre-event intensity and beta derivative.
// Side selection is routing between parameter records; support and arithmetic
// are shared compositions. Input uses flat natural parameter names and events.
func NewIntensity(side core.Primitive) core.Primitive {
	return transport.NewPipe(
		store.NewRecord(
			transport.NewPipe(),
			logic.NewGate(
				equation.NewEqual[float64](side, store.NewConstant(core.From(0.0))),
				store.NewRecord(
					transport.NewPipe(store.NewGet("mu_x"), store.NewKey("mu")),
					transport.NewPipe(store.NewGet("alpha_xx"), store.NewKey("alpha_x")),
					transport.NewPipe(store.NewGet("alpha_xy"), store.NewKey("alpha_y")),
				),
				store.NewRecord(
					transport.NewPipe(store.NewGet("mu_y"), store.NewKey("mu")),
					transport.NewPipe(store.NewGet("alpha_yx"), store.NewKey("alpha_x")),
					transport.NewPipe(store.NewGet("alpha_yy"), store.NewKey("alpha_y")),
				),
			),
		),
		store.NewRecord(
			transport.NewPipe(),
			transport.NewPipe(
				NewSupport(store.NewConstant(core.From(0.0)), NewKernel(store.NewGet("beta"), store.NewGet("age"))),
				store.NewKey("support_x"),
			),
			transport.NewPipe(
				NewSupport(store.NewConstant(core.From(1.0)), NewKernel(store.NewGet("beta"), store.NewGet("age"))),
				store.NewKey("support_y"),
			),
			transport.NewPipe(
				NewSupport(
					store.NewConstant(core.From(0.0)),
					transport.NewPipe(
						equation.NewProduct[float64](store.NewGet("age"), NewKernel(store.NewGet("beta"), store.NewGet("age"))),
						calculus.NewNegate(transport.NewIO(core.From(0.0))),
					),
				),
				store.NewKey("support_x_beta"),
			),
			transport.NewPipe(
				NewSupport(
					store.NewConstant(core.From(1.0)),
					transport.NewPipe(
						equation.NewProduct[float64](store.NewGet("age"), NewKernel(store.NewGet("beta"), store.NewGet("age"))),
						calculus.NewNegate(transport.NewIO(core.From(0.0))),
					),
				),
				store.NewKey("support_y_beta"),
			),
		),
		store.NewRecord(
			transport.NewPipe(),
			transport.NewPipe(
				equation.NewSum[float64](
					store.NewGet("mu"),
					equation.NewSum[float64](
						equation.NewProduct[float64](store.NewGet("alpha_x"), store.NewGet("support_x")),
						equation.NewProduct[float64](store.NewGet("alpha_y"), store.NewGet("support_y")),
					),
				),
				store.NewKey("intensity"),
			),
			transport.NewPipe(
				equation.NewSum[float64](
					equation.NewProduct[float64](store.NewGet("alpha_x"), store.NewGet("support_x_beta")),
					equation.NewProduct[float64](store.NewGet("alpha_y"), store.NewGet("support_y_beta")),
				),
				store.NewKey("intensity_beta"),
			),
		),
	)
}
