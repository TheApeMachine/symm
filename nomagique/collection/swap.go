package collection

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

// NewSwap is two indexed replacements over one captured collection. It adds
// no second swapping kernel; both original operands survive the first write.
func NewSwap[T any](left, right core.Primitive) core.Primitive {
	values := store.NewRetained(core.From([]T{}))
	first := store.NewRetained(core.From(0.0))
	second := store.NewRetained(core.From(0.0))
	return transport.NewPipe(
		transport.NewFan(
			transport.NewPipe(),
			transport.NewIO(
				transport.NewPipe(transport.NewApply(first, left), transport.NewDiscard()),
				transport.NewPipe(transport.NewApply(second, right), transport.NewDiscard()),
				transport.NewPipe(),
			),
		),
		values,
		NewSet[T](first, transport.NewApply(NewAt[T](second), values)),
		NewSet[T](second, transport.NewApply(NewAt[T](first), values)),
	)
}
