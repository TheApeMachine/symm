package equation

import (
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
NewRenewalRate accumulates quantity until a configured target is reached and
positive time has elapsed. Input records contain increment and sample (float64)
and at (int64 nanoseconds). The target receives each increment and owns its
estimation policy. It must yield a positive quantity; no window is selected here.

Each observation yields a record. closed distinguishes a completed span from a
retained prior rate. The first completed span has no prior sample for change.
*/
func NewRenewalRate(target core.Primitive) core.Primitive {
	memory := store.NewRetained(core.From(map[string]core.Primitive{
		"accumulated": core.From(0.0), "spans": core.From(0.0),
		"rate": core.From(0.0), "change": core.From(0.0),
		"maturity": core.From(0.0), "closed": core.From(false),
	}))
	state := transport.NewPipe(store.NewKV[string](memory), memory)
	initialize := logic.NewGate(
		store.NewHas("origin"), transport.NewPipe(),
		store.NewRecord(transport.NewPipe(), transport.NewPipe(store.NewGet("at"), store.NewKey("origin"))),
	)
	closeSpan := transport.NewPipe(
		store.NewRecord(
			transport.NewPipe(),
			transport.NewPipe(NewRatio[float64](store.NewGet("accumulated"), store.NewGet("elapsed")), store.NewKey("rate")),
			transport.NewPipe(
				logic.NewGate(
					store.NewHas("last_sample"),
					transport.NewPipe(
						NewRatio[float64](store.NewGet("sample"), store.NewGet("last_sample")),
						calculus.NewLog(transport.NewIO(core.From(0.0))),
					),
					store.NewConstant(core.From(0.0)),
				),
				store.NewKey("change"),
			),
			transport.NewPipe(NewSum[float64](store.NewGet("spans"), store.NewConstant(core.From(1.0))), store.NewKey("spans")),
			transport.NewPipe(store.NewGet("sample"), store.NewKey("last_sample")),
			transport.NewPipe(store.NewGet("at"), store.NewKey("origin")),
			transport.NewPipe(store.NewConstant(core.From(0.0)), store.NewKey("accumulated")),
			transport.NewPipe(store.NewConstant(core.From(true)), store.NewKey("closed")),
		),
		store.NewRecord(
			transport.NewPipe(),
			transport.NewPipe(
				NewRatio[float64](store.NewGet("spans"), NewSum[float64](store.NewGet("spans"), store.NewConstant(core.From(1.0)))),
				store.NewKey("maturity"),
			),
		),
	)

	transition := transport.NewPipe(
		state,
		initialize,
		store.NewRecord(
			transport.NewPipe(),
			transport.NewPipe(store.NewGet("increment"), target, store.NewKey("target")),
			transport.NewPipe(NewSum[float64](store.NewGet("accumulated"), store.NewGet("increment")), store.NewKey("accumulated")),
			transport.NewPipe(NewElapsed(store.NewGet("origin"), store.NewGet("at")), store.NewKey("elapsed")),
			transport.NewPipe(store.NewConstant(core.From(false)), store.NewKey("closed")),
		),
		logic.NewGate(
			NewAll(
				NewGreater[float64](store.NewGet("target"), store.NewConstant(core.From(0.0))),
				NewGreater[float64](store.NewGet("sample"), store.NewConstant(core.From(0.0))),
				NewLessEqual[float64](store.NewConstant(core.From(0.0)), store.NewGet("increment")),
				NewLessEqual[float64](store.NewConstant(core.From(0.0)), store.NewGet("elapsed")),
			),
			transport.NewPipe(
				logic.NewGate(
					NewAll(
						NewLessEqual[float64](store.NewGet("target"), store.NewGet("accumulated")),
						NewGreater[float64](store.NewGet("elapsed"), store.NewConstant(core.From(0.0))),
					),
					closeSpan, transport.NewPipe(),
				),
				state,
			),
			logic.NewReject(core.ErrDomain),
		),
	)

	return transport.NewMap(logic.NewGate(
		NewAll(store.NewHas("increment"), store.NewHas("sample"), store.NewHas("at")),
		transition,
		logic.NewReject(core.ErrShape),
	))
}
