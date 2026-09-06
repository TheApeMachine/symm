package equation

import (
	"github.com/theapemachine/symm/nomagique/arithmetic"
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

// NewLagProfile evaluates a configured dependence estimator at each lag. Spacing
// is nanoseconds; span is an integer number of steps to either side. Both are
// Primitive sources and may themselves be measured compositions. The estimator
// is supplied; this package does not own or duplicate its algorithm.
//
// Every candidate retains the complete estimator record, including its own
// overlap support. x is lag seconds and y is correlation for profile analysis.
func NewLagProfile(estimator, spacing, span core.Primitive) core.Primitive {
	state := store.NewRetained(core.From(map[string]core.Primitive{}))
	lag := store.NewRetained(core.From(int64(0)))
	return transport.NewPipe(
		state,
		transport.NewRange(
			transport.NewPipe(
				transport.NewApply(span, nil),
				arithmetic.NewMultiply[float64](transport.NewIO(core.From(2.0))),
				arithmetic.NewAdd[float64](transport.NewIO(core.From(1.0))),
			),
		),
		transport.NewMap(
			transport.NewPipe(
				NewDifference[float64](transport.NewPipe(), transport.NewApply(span, nil)),
				NewProduct[float64](transport.NewPipe(), transport.NewApply(spacing, nil)),
				calculus.NewConvert[float64, int64](),
				lag,
				transport.NewApply(state, nil),
				store.NewRecord(
					transport.NewPipe(
						store.NewGet("left"),
						transport.NewSpread[core.Primitive](),
						transport.NewMap(
							store.NewRecord(
								transport.NewPipe(NewSum[int64](store.NewGet("at"), transport.NewApply(lag, nil)), store.NewKey("at")),
								transport.NewPipe(store.NewGet("value"), store.NewKey("value")),
							),
						),
						transport.NewCollect[core.Primitive](),
						store.NewKey("left"),
					),
					transport.NewPipe(store.NewGet("right"), store.NewKey("right")),
				),
				estimator,
				store.NewRecord(
					transport.NewPipe(),
					transport.NewPipe(
						transport.NewApply(lag, nil),
						calculus.NewConvert[int64, float64](),
						arithmetic.NewMultiply[float64](transport.NewIO(core.From(1e-9))),
						store.NewKey("x"),
					),
					transport.NewPipe(store.NewGet("correlation"), store.NewKey("y")),
				),
			),
		),
	)
}
