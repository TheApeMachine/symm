package correlation

import (
	"github.com/theapemachine/symm/nomagique/arithmetic"
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/collection"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

// NewDependence adds the source signal's path diagnostics around a configured
// estimator. The estimator is opaque; normally it is algo.NewHayashiYoshida().
// Correlation is not clipped. Definedness is explicit and belongs to this
// reporting composition, not to arithmetic or the HY covariance calculation.
func NewDependence(estimator core.Primitive) core.Primitive {
	input := store.NewRetained(nil)
	statistics := transport.NewPipe(
		input,
		store.NewRecord(transport.NewPipe(),
			transport.NewPipe(estimator, store.NewKey("estimate"))),
		store.NewRecord(
			store.NewGet("estimate"),
			transport.NewPipe(store.NewGet("left"), transport.NewSpread[core.Primitive](),
				equation.NewLogReturns(), equation.NewCount(), store.NewKey("left_returns")),
			transport.NewPipe(store.NewGet("right"), transport.NewSpread[core.Primitive](),
				equation.NewLogReturns(), equation.NewCount(), store.NewKey("right_returns")),
			transport.NewPipe(store.NewGet("left"), transport.NewSpread[core.Primitive](),
				equation.NewLogReturns(), equation.NewEnergyRates(),
				logic.NewGate(equation.NewGreater[float64](equation.NewCount(), store.NewConstant(core.From(0.0))),
					equation.NewMedian(), equation.NewRatio[float64](store.NewConstant(core.From(0.0)), store.NewConstant(core.From(0.0)))), store.NewKey("left_energy_rate")),
			transport.NewPipe(store.NewGet("right"), transport.NewSpread[core.Primitive](),
				equation.NewLogReturns(), equation.NewEnergyRates(),
				logic.NewGate(equation.NewGreater[float64](equation.NewCount(), store.NewConstant(core.From(0.0))),
					equation.NewMedian(), equation.NewRatio[float64](store.NewConstant(core.From(0.0)), store.NewConstant(core.From(0.0)))), store.NewKey("right_energy_rate")),
		),
		store.NewRecord(transport.NewPipe(),
			transport.NewPipe(equation.NewAll(
				equation.NewGreater[float64](store.NewGet("support"), store.NewConstant(core.From(0.0))),
				equation.NewGreater[float64](store.NewGet("left_energy"), store.NewConstant(core.From(0.0))),
				equation.NewGreater[float64](store.NewGet("right_energy"), store.NewConstant(core.From(0.0))),
				transport.NewPipe(store.NewGet("correlation"), logic.NewFinite())), store.NewKey("defined"))),
	)
	// Integer comparison and selection preserve full nanosecond coordinates.
	leftEnd := transport.NewPipe(store.NewGet("left"), collection.NewTail[core.Primitive](store.NewConstant(core.From(1))),
		transport.NewSpread[core.Primitive](), store.NewGet("at"))
	rightEnd := transport.NewPipe(store.NewGet("right"), collection.NewTail[core.Primitive](store.NewConstant(core.From(1))),
		transport.NewSpread[core.Primitive](), store.NewGet("at"))
	leftStart := transport.NewPipe(store.NewGet("left"), collection.NewAt[core.Primitive](transport.NewIO(core.From(0.0))), store.NewGet("at"))
	rightStart := transport.NewPipe(store.NewGet("right"), collection.NewAt[core.Primitive](transport.NewIO(core.From(0.0))), store.NewGet("at"))
	shared := transport.NewPipe(
		transport.NewApply(input, nil),
		equation.NewDifference[int64](
			logic.NewGate(equation.NewLess[int64](leftEnd, rightEnd), leftEnd, rightEnd),
			logic.NewGate(equation.NewLess[int64](leftStart, rightStart), rightStart, leftStart)),
		calculus.NewConvert[int64, float64](),
		calculus.NewMaximum(transport.NewIO(core.From(0.0))),
		arithmetic.NewMultiply[float64](transport.NewIO(core.From(1e-9))),
	)
	return transport.NewPipe(
		statistics,
		store.NewRecord(transport.NewPipe(),
			transport.NewPipe(logic.NewGate(equation.NewAll(
				equation.NewGreater[float64](store.NewGet("left_returns"), store.NewConstant(core.From(0.0))),
				equation.NewGreater[float64](store.NewGet("right_returns"), store.NewConstant(core.From(0.0)))),
				shared, store.NewConstant(core.From(0.0))), store.NewKey("shared_time"))),
		store.NewRecord(transport.NewPipe(),
			transport.NewPipe(logic.NewGate(
				equation.NewGreater[float64](store.NewGet("shared_time"), store.NewConstant(core.From(0.0))),
				equation.NewRatio[float64](store.NewGet("support"), store.NewGet("shared_time")),
				store.NewConstant(core.From(0.0))), store.NewKey("overlap_density"))),
	)
}
