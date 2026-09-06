package equation

import (
	"cmp"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/transport"
)

func NewGreater[T cmp.Ordered](left, right core.Primitive) core.Primitive {
	return transport.NewPipe(
		transport.NewFan(transport.NewPipe(), transport.NewIO(left, right)),
		transport.NewCollect[T](),
		logic.NewGreater[T](),
	)
}
