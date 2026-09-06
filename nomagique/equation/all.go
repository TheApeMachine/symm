package equation

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/transport"
)

// NewAll combines predicates over the same captured input. Every predicate is
// evaluated; the empty conjunction is true.
func NewAll(predicates ...core.Primitive) core.Primitive {
	return transport.NewPipe(
		transport.NewFan(transport.NewPipe(), transport.NewIO(predicates...)),
		logic.NewAnd(transport.NewIO(core.From(true))),
	)
}
