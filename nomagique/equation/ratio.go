package equation

import (
	"github.com/theapemachine/symm/nomagique/arithmetic"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

// NewRatio binds two expressions to one captured input run. Both operands are
// computations, not decoded values; the source is consumed only once.
func NewRatio[T core.Floating](left, right core.Primitive) core.Primitive {
	captured := store.NewRetained(core.From([]core.Primitive{}))
	return transport.NewPipe(
		transport.NewCollect[core.Primitive](),
		captured,
		transport.NewApply(
			arithmetic.NewDivide[T](transport.NewApply(transport.NewPipe(transport.NewSpread[core.Primitive](), left), captured)),
			transport.NewApply(transport.NewPipe(transport.NewSpread[core.Primitive](), right), captured),
		),
	)
}
