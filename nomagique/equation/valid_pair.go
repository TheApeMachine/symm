package equation

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

// NewValidPair is the supplied prediction/actual domain: finite, nonzero values.
func NewValidPair() core.Primitive {
	return NewAll(
		transport.NewPipe(store.NewGet("predicted"), logic.NewFinite()),
		transport.NewPipe(store.NewGet("actual"), logic.NewFinite()),
		transport.NewPipe(
			NewEqual[float64](store.NewGet("predicted"), store.NewConstant(core.From(0.0))),
			logic.NewNot(transport.NewIO(core.From(false))),
		),
		transport.NewPipe(
			NewEqual[float64](store.NewGet("actual"), store.NewConstant(core.From(0.0))),
			logic.NewNot(transport.NewIO(core.From(false))),
		),
	)
}
