package joint

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
	"github.com/theapemachine/symm/nomagique/vector"
)

/*
NewDivergence composes joint causal moments with one residual regression per
coordinate. Input contains values ([]float64 in log space) and at (int64
nanoseconds). Output retains the joint record and adds velocities, in channel
order. The supplied endpoint collections own shape and per-channel history.
Each stream requires its own composition; no hidden key selector owns state.
Regressing event time is rejected before any estimator observes the record.
*/
func NewDivergence(estimators, regressions core.Primitive) core.Primitive {
	observation := store.NewRetained(core.From(map[string]core.Primitive{}))
	chronological := logic.NewGate(
		transport.NewPipe(transport.NewApply(observation, nil), store.NewHas("at")),
		equation.NewLessEqual[int64](
			transport.NewPipe(transport.NewApply(observation, nil), store.NewGet("at")),
			store.NewGet("at"),
		),
		store.NewConstant(core.From(true)),
	)
	velocities := transport.NewPipe(
		store.NewGet("channels"),
		transport.NewSpread[core.Primitive](),
		transport.NewMap(store.NewRecord(
			transport.NewPipe(store.NewGet("residual"), store.NewKey("value")),
			transport.NewPipe(transport.NewApply(observation, nil), store.NewGet("at"), store.NewKey("at")),
		)),
		vector.NewApply(regressions),
		transport.NewCollect[core.Primitive](),
	)

	return transport.NewMap(transport.NewPipe(
		store.NewRecord(
			transport.NewPipe(),
			transport.NewPipe(store.NewGet("at"), store.NewKey("at")),
			transport.NewPipe(store.NewGet("values"), store.NewKey("values")),
		),
		logic.NewGate(
			chronological,
			transport.NewPipe(
				observation,
				NewEstimator(estimators),
				store.NewRecord(transport.NewPipe(), transport.NewPipe(velocities, store.NewKey("velocities"))),
			),
			logic.NewReject(core.ErrDomain),
		),
	))
}
