package algo

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

// NewWelford wires Welford's running-moment recurrence through one retained KV.
// Optional record-to-record policies may transform support after an update,
// before variance is derived. With no policy this is cumulative Welford.
// Prior fields always describe the moments before the current observation.
func NewWelford(policies ...core.Primitive) core.Primitive {
	memory := store.NewRetained(core.From(map[string]core.Primitive{
		"count": core.From(0.0), "mean": core.From(0.0), "m2": core.From(0.0),
	}))
	state := transport.NewPipe(store.NewKV[string](memory), memory)
	return transport.NewMap(
		transport.NewPipe(
			store.NewKey("value"),
			state,
			equation.NewMomentUpdate(),
			state,
			transport.NewPipe(policies...),
			state,
			equation.NewMomentSummary(),
			state,
		),
	)
}
