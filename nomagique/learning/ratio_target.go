package learning

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

// NewRatioTarget is the relative change, with an explicit nonzero past domain.
func NewRatioTarget() core.Primitive {
	return logic.NewGate(
		equation.NewAll(
			equation.NewAll(
				transport.NewPipe(store.NewGet("current"), logic.NewFinite()),
				transport.NewPipe(store.NewGet("past"), logic.NewFinite()),
			),
			transport.NewPipe(
				equation.NewEqual[float64](store.NewGet("past"), store.NewConstant(core.From(0.0))),
				logic.NewNot(transport.NewIO(core.From(false))),
			),
		),
		equation.NewDifference[float64](
			equation.NewRatio[float64](store.NewGet("current"), store.NewGet("past")),
			store.NewConstant(core.From(1.0)),
		),
		logic.NewReject(core.ErrDomain),
	)
}
