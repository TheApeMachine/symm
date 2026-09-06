package learning

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

// NewBinaryTarget classifies an increase without inventing a new numeric rule.
func NewBinaryTarget() core.Primitive {
	return logic.NewGate(
		equation.NewAll(
			transport.NewPipe(store.NewGet("current"), logic.NewFinite()),
			transport.NewPipe(store.NewGet("past"), logic.NewFinite()),
		),
		logic.NewGate(
			equation.NewGreater[float64](store.NewGet("current"), store.NewGet("past")),
			store.NewConstant(core.From(1.0)),
			store.NewConstant(core.From(0.0)),
		),
		logic.NewReject(core.ErrDomain),
	)
}
