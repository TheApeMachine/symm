package equation

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/transport"
)

// NewAny combines predicates over the same captured input. The empty
// disjunction is false.
func NewAny(predicates ...core.Primitive) core.Primitive {
	return transport.NewPipe(
		transport.NewFan(transport.NewPipe(), transport.NewIO(predicates...)),
		logic.NewOr(transport.NewIO(core.From(false))),
	)
}
