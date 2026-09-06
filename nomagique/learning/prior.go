package learning

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

// NewPrior composes storage, aging, weighted moments and readout. An observation
// is a KV record with value and authority; an optional uint64 epoch selects its
// causal clock. An epoch-only record queries/ages without inventing a sample.
// No epoch means one aging step per positive-authority observation. Zero
// authority increments samples, but does not create evidence or age it.
func NewPrior(memorySize core.Primitive) core.Primitive {
	memory := store.NewRetained(
		core.From(
			map[string]core.Primitive{
				"samples": core.From(uint64(0)), "pending": core.From(uint64(0)),
				"weight": core.From(0.0), "support": core.From(0.0), "moment": core.From(0.0),
				"mean": core.From(0.0), "last_epoch": core.From(uint64(0)),
			},
		),
	)
	state := transport.NewPipe(store.NewKV[string](memory), memory)
	age := logic.NewGate(
		store.NewGet("epoch_supplied"),
		equation.NewPriorAge(),
		transport.NewPipe(
			store.NewRecord(transport.NewPipe(), transport.NewPipe(store.NewConstant(core.From(1.0)), store.NewKey("gap"))),
			store.NewRecord(
				transport.NewPipe(),
				transport.NewPipe(equation.NewProduct[float64](store.NewGet("weight"), equation.NewRetentionFactor()), store.NewKey("weight")),
			),
		),
	)
	observe := logic.NewGate(
		equation.NewAll(
			transport.NewPipe(store.NewGet("authority"), logic.NewFinite()),
			equation.NewLessEqual[float64](store.NewConstant(core.From(0.0)), store.NewGet("authority")),
			equation.NewLessEqual[float64](store.NewGet("authority"), store.NewConstant(core.From(1.0))),
		),
		transport.NewPipe(
			state,
			store.NewRecord(
				transport.NewPipe(),
				transport.NewPipe(
					equation.NewSum[uint64](store.NewGet("samples"), store.NewConstant(core.From(uint64(1)))),
					store.NewKey("samples"),
				),
			),
			logic.NewGate(
				equation.NewGreater[float64](store.NewGet("authority"), store.NewConstant(core.From(0.0))),
				transport.NewPipe(age, equation.NewPriorUpdate()),
				transport.NewPipe(),
			),
			state,
			equation.NewPriorSummary(),
		),
		logic.NewReject(core.ErrDomain),
	)
	query := transport.NewPipe(
		state,
		logic.NewGate(store.NewGet("epoch_supplied"), equation.NewPriorAge(), transport.NewPipe()),
		state,
		equation.NewPriorSummary(),
	)
	return transport.NewMap(
		transport.NewPipe(
			store.NewRecord(
				transport.NewPipe(),
				transport.NewPipe(memorySize, store.NewKey("memory")),
				transport.NewPipe(store.NewHas("epoch"), store.NewKey("epoch_supplied")),
			),
			logic.NewGate(store.NewHas("value"), observe, query),
		),
	)
}
