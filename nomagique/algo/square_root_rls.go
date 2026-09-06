package algo

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/equation/rls"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
	"github.com/theapemachine/symm/nomagique/vector"
)

// NewSquareRootRLS composes recursive least squares over a configured initial
// coefficient/root record. Each {design,target} is predicted before training;
// {design} alone predicts without updating. Absence is tested on this input,
// never against an old merged target. Only successful updates commit model state.
// Invalid samples report errors; unlike the supplied learner, no silent reset
// and retry is hidden inside the numerical operation.
func NewSquareRootRLS(initial, forgetting core.Primitive) core.Primitive {
	memory := store.NewRetained(core.From(map[string]core.Primitive{}))
	snapshot := store.NewRecord(
		transport.NewPipe(store.NewGet("beta"), store.NewKey("beta")),
		transport.NewPipe(store.NewGet("root"), store.NewKey("root")),
		transport.NewPipe(store.NewGet("noise_shape"), store.NewKey("noise_shape")),
		transport.NewPipe(store.NewGet("noise_scale"), store.NewKey("noise_scale")),
	)
	commit := transport.NewFan(
		transport.NewPipe(),
		transport.NewIO(transport.NewPipe(snapshot, memory, transport.NewDiscard()), transport.NewPipe()),
	)
	return transport.NewMap(
		transport.NewPipe(
			store.NewRecord(
				transport.NewPipe(),
				transport.NewPipe(store.NewHas("target"), store.NewKey("observe")),
				transport.NewPipe(forgetting, store.NewKey("lambda")),
			),
			store.NewKV[string](memory),
			logic.NewGate(store.NewHas("beta"), transport.NewPipe(), store.NewRecord(initial, transport.NewPipe())),
			logic.NewGate(
				equation.NewAll(
					equation.NewGreater[float64](store.NewGet("lambda"), store.NewConstant(core.From(0.0))),
					equation.NewLessEqual[float64](store.NewGet("lambda"), store.NewConstant(core.From(1.0))),
					transport.NewPipe(store.NewGet("design"), transport.NewSpread[float64](), vector.NewFinite()),
				),
				transport.NewPipe(
					rls.NewPrediction(store.NewConstant(core.From(1.0))),
					logic.NewGate(store.NewGet("observe"), transport.NewPipe(rls.NewUpdate(), commit), transport.NewPipe()),
				),
				logic.NewReject(core.ErrDomain),
			),
		),
	)
}
