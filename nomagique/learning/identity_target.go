package learning

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

// NewIdentityTarget selects the finite current value.
func NewIdentityTarget() core.Primitive {
	return logic.NewGate(
		transport.NewPipe(store.NewGet("current"), logic.NewFinite()),
		store.NewGet("current"),
		logic.NewReject(core.ErrDomain),
	)
}
