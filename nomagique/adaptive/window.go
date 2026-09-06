package adaptive

import (
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

// NewWindow composes the supplied adaptive/window.go mean-shift policy. It
// preserves that policy's all/recent moment approximation; it is not a new
// implementation or statistical validation of the full ADWIN algorithm.
// One numeric observation yields capacity, shed_ratio and the moment facts.
func NewWindow() core.Primitive {
	empty := core.From(map[string]core.Primitive{
		"count": core.From(0.0), "mean": core.From(0.0), "m2": core.From(0.0),
	})
	memory := store.NewRetained(
		core.From(
			map[string]core.Primitive{
				"all": empty, "recent": empty, "capacity": core.From(2.0),
				"observations": core.From(0.0), "shed_ratio": core.From(1.0),
			},
		),
	)
	state := transport.NewPipe(store.NewKV[string](memory), memory)

	updateAll := transport.NewPipe(
		store.NewRecord(
			store.NewGet("all"), transport.NewPipe(store.NewGet("value"), store.NewKey("value"))),
		equation.NewMomentUpdate(),
		equation.NewMomentSummary(),
		store.NewKey("all"),
	)
	updateRecent := transport.NewPipe(
		store.NewRecord(
			store.NewGet("recent"), transport.NewPipe(store.NewGet("value"), store.NewKey("value"))),
		equation.NewMomentUpdate(),
		equation.NewMomentSummary(),
		store.NewKey("recent"),
	)

	trimRecent := logic.NewGate(
		equation.NewAll(
			equation.NewGreater[float64](store.NewGet("observations"), store.NewConstant(core.From(3.0))),
			equation.NewGreater[float64](
				transport.NewPipe(store.NewGet("recent"), store.NewGet("count")),
				equation.NewProduct[float64](store.NewGet("capacity"), store.NewConstant(core.From(0.5))),
			),
		),
		store.NewRecord(
			transport.NewPipe(),
			transport.NewPipe(
				store.NewGet("recent"),
				store.NewRecord(transport.NewPipe(), transport.NewPipe(store.NewConstant(core.From(0.5)), store.NewKey("retain"))),
				equation.NewShedMoments(),
				equation.NewMomentSummary(),
				store.NewKey("recent"),
			),
		),
		transport.NewPipe(),
	)

	contract := transport.NewPipe(
		store.NewRecord(
			transport.NewPipe(),
			transport.NewPipe(
				equation.NewProduct[float64](store.NewGet("capacity"), store.NewConstant(core.From(0.5))),
				calculus.NewFloor(transport.NewIO(core.From(0.0))),
				calculus.NewMaximum(transport.NewIO(core.From(2.0))),
				store.NewKey("new_capacity"),
			),
		),
		store.NewRecord(
			transport.NewPipe(),
			transport.NewPipe(
				equation.NewRatio[float64](store.NewGet("new_capacity"), store.NewGet("capacity")), store.NewKey("shed_ratio")),
		),
		store.NewRecord(
			transport.NewPipe(),
			transport.NewPipe(store.NewGet("new_capacity"), store.NewKey("capacity")),
			transport.NewPipe(
				store.NewRecord(store.NewGet("all"), transport.NewPipe(store.NewGet("shed_ratio"), store.NewKey("retain"))),
				equation.NewShedMoments(),
				equation.NewMomentSummary(),
				store.NewKey("all"),
			),
			transport.NewPipe(store.NewConstant(empty), store.NewKey("recent")),
		),
	)

	inspect := transport.NewPipe(
		store.NewRecord(
			transport.NewPipe(),
			transport.NewPipe(store.NewGet("all"), store.NewGet("variance"), store.NewKey("variance")),
			transport.NewPipe(store.NewGet("recent"), store.NewGet("count"), store.NewKey("recent_count")),
		),
		store.NewRecord(
			transport.NewPipe(),
			transport.NewPipe(
				equation.NewDifference[float64](store.NewGet("capacity"), store.NewGet("recent_count")),
				store.NewKey("prior_count"),
			),
		),
		logic.NewGate(
			equation.NewAll(
				equation.NewGreater[float64](store.NewGet("observations"), store.NewConstant(core.From(3.0))),
				equation.NewGreater[float64](store.NewGet("recent_count"), store.NewConstant(core.From(1.0))),
				equation.NewGreater[float64](store.NewGet("prior_count"), store.NewConstant(core.From(1.0))),
				equation.NewGreater[float64](store.NewGet("variance"), store.NewConstant(core.From(0.0))),
			),
			logic.NewGate(
				equation.NewGreater[float64](
					transport.NewPipe(
						equation.NewDifference[float64](
							transport.NewPipe(store.NewGet("recent"), store.NewGet("mean")),
							transport.NewPipe(store.NewGet("all"), store.NewGet("mean")),
						),
						calculus.NewAbsolute(transport.NewIO(core.From(0.0))),
					),
					equation.NewMeanShiftBound(),
				),
				contract,
				transport.NewPipe(),
			),
			transport.NewPipe(),
		),
	)

	return transport.NewMap(
		transport.NewPipe(
			store.NewKey("value"),
			state,
			store.NewRecord(
				transport.NewPipe(),
				updateAll,
				updateRecent,
				transport.NewPipe(
					equation.NewSum[float64](store.NewGet("observations"), store.NewConstant(core.From(1.0))),
					store.NewKey("observations"),
				),
				transport.NewPipe(equation.NewSum[float64](store.NewGet("capacity"), store.NewConstant(core.From(1.0))), store.NewKey("capacity")),
				transport.NewPipe(store.NewConstant(core.From(1.0)), store.NewKey("shed_ratio")),
			),
			trimRecent,
			inspect,
			state,
		),
	)
}
