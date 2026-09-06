package learning

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

// NewDeltaTarget returns the observed current-minus-past difference.
func NewDeltaTarget() core.Primitive {
	return logic.NewGate(
		equation.NewAll(
			transport.NewPipe(store.NewGet("current"), logic.NewFinite()),
			transport.NewPipe(store.NewGet("past"), logic.NewFinite()),
		),
		equation.NewDifference[float64](store.NewGet("current"), store.NewGet("past")),
		logic.NewReject(core.ErrDomain),
	)
}
