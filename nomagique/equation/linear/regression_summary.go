package linear

import (
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

// NewRegressionSummary composes the supplied cumulative slope and residual
// SNR. Each intermediate is calculated once. The raw candidates remain visible
// even when their domain fails; published slope/SNR use the original zero plus
// false convention. No ridge or stable centered-update algorithm is substituted.
func NewRegressionSummary() core.Primitive {
	means := store.NewRecord(transport.NewPipe(),
		transport.NewPipe(equation.NewRatio[float64](store.NewGet("sum_x"), store.NewGet("count")), store.NewKey("mean_x")),
		transport.NewPipe(equation.NewRatio[float64](store.NewGet("sum_y"), store.NewGet("count")), store.NewKey("mean_y")),
	)
	centered := store.NewRecord(transport.NewPipe(),
		transport.NewPipe(equation.NewDifference[float64](store.NewGet("sum_xx"),
			equation.NewProduct[float64](equation.NewProduct[float64](store.NewGet("count"), store.NewGet("mean_x")), store.NewGet("mean_x"))), store.NewKey("sxx")),
		transport.NewPipe(equation.NewDifference[float64](store.NewGet("sum_xy"),
			equation.NewProduct[float64](equation.NewProduct[float64](store.NewGet("count"), store.NewGet("mean_x")), store.NewGet("mean_y"))), store.NewKey("sxy")),
		transport.NewPipe(equation.NewDifference[float64](store.NewGet("sum_yy"),
			equation.NewProduct[float64](equation.NewProduct[float64](store.NewGet("count"), store.NewGet("mean_y")), store.NewGet("mean_y"))), store.NewKey("syy")),
	)
	fit := transport.NewPipe(
		store.NewRecord(transport.NewPipe(), transport.NewPipe(
			equation.NewRatio[float64](store.NewGet("sxy"), store.NewGet("sxx")), store.NewKey("raw_slope"))),
		store.NewRecord(transport.NewPipe(), transport.NewPipe(equation.NewDifference[float64](store.NewGet("syy"),
			equation.NewProduct[float64](store.NewGet("raw_slope"), store.NewGet("sxy"))), store.NewKey("sse"))),
		store.NewRecord(transport.NewPipe(), transport.NewPipe(equation.NewRatio[float64](store.NewGet("sse"),
			equation.NewDifference[float64](store.NewGet("count"), store.NewConstant(core.From(2.0)))), store.NewKey("residual_variance"))),
		store.NewRecord(transport.NewPipe(), transport.NewPipe(equation.NewRatio[float64](store.NewGet("residual_variance"), store.NewGet("sxx")), store.NewKey("slope_variance"))),
		store.NewRecord(transport.NewPipe(), transport.NewPipe(equation.NewRatio[float64](
			transport.NewPipe(store.NewGet("raw_slope"), calculus.NewSquare(transport.NewIO(core.From(0.0)))), store.NewGet("slope_variance")), store.NewKey("raw_snr"))),
	)
	domains := store.NewRecord(transport.NewPipe(),
		transport.NewPipe(equation.NewAll(
			equation.NewLessEqual[float64](store.NewConstant(core.From(3.0)), store.NewGet("count")),
			equation.NewGreater[float64](store.NewGet("sxx"), store.NewConstant(core.From(0.0))),
			transport.NewPipe(store.NewGet("raw_slope"), logic.NewFinite()),
		), store.NewKey("slope_defined")),
		transport.NewPipe(equation.NewAll(
			equation.NewLessEqual[float64](store.NewConstant(core.From(4.0)), store.NewGet("count")),
			equation.NewGreater[float64](store.NewGet("sxx"), store.NewConstant(core.From(0.0))),
			equation.NewGreater[float64](store.NewGet("syy"), store.NewConstant(core.From(0.0))),
			equation.NewGreater[float64](store.NewGet("sse"), store.NewConstant(core.From(0.0))),
			equation.NewGreater[float64](store.NewGet("residual_variance"), store.NewConstant(core.From(0.0))),
			transport.NewPipe(store.NewGet("raw_snr"), logic.NewFinite()),
		), store.NewKey("snr_defined")),
	)
	return transport.NewPipe(means, centered, fit, domains,
		store.NewRecord(transport.NewPipe(),
			transport.NewPipe(logic.NewGate(store.NewGet("slope_defined"), store.NewGet("raw_slope"), store.NewConstant(core.From(0.0))), store.NewKey("slope")),
			transport.NewPipe(logic.NewGate(store.NewGet("snr_defined"), store.NewGet("raw_snr"), store.NewConstant(core.From(0.0))), store.NewKey("snr")),
		),
	)
}
