package linear

import (
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

// NewRegressionMoments retains the five normal-equation sums and count of
// scalar (x,y) observations. It owns no regression solver or admission window;
// arithmetic performs each update and the configured KV feedback retains it.
func NewRegressionMoments() core.Primitive {
	memory := store.NewRetained(
		core.From(
			map[string]core.Primitive{
				"count": core.From(0.0), "sum_x": core.From(0.0), "sum_y": core.From(0.0),
				"sum_xx": core.From(0.0), "sum_xy": core.From(0.0), "sum_yy": core.From(0.0),
			},
		),
	)
	state := transport.NewPipe(store.NewKV[string](memory), memory)
	return transport.NewMap(
		logic.NewGate(
			equation.NewAll(transport.NewPipe(store.NewGet("x"), logic.NewFinite()), transport.NewPipe(store.NewGet("y"), logic.NewFinite())),
			transport.NewPipe(
				state,
				store.NewRecord(
					transport.NewPipe(),
					transport.NewPipe(equation.NewSum[float64](store.NewGet("count"), store.NewConstant(core.From(1.0))), store.NewKey("count")),
					transport.NewPipe(equation.NewSum[float64](store.NewGet("sum_x"), store.NewGet("x")), store.NewKey("sum_x")),
					transport.NewPipe(equation.NewSum[float64](store.NewGet("sum_y"), store.NewGet("y")), store.NewKey("sum_y")),
					transport.NewPipe(
						equation.NewSum[float64](
							store.NewGet("sum_xx"),
							transport.NewPipe(store.NewGet("x"), calculus.NewSquare(transport.NewIO(core.From(0.0)))),
						),
						store.NewKey("sum_xx"),
					),
					transport.NewPipe(
						equation.NewSum[float64](store.NewGet("sum_xy"), equation.NewProduct[float64](store.NewGet("x"), store.NewGet("y"))),
						store.NewKey("sum_xy"),
					),
					transport.NewPipe(
						equation.NewSum[float64](
							store.NewGet("sum_yy"),
							transport.NewPipe(store.NewGet("y"), calculus.NewSquare(transport.NewIO(core.From(0.0)))),
						),
						store.NewKey("sum_yy"),
					),
				),
				state,
			),
			logic.NewReject(core.ErrDomain),
		),
	)
}
