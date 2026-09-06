package arithmetic

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

// Subtract owns one field operation. Configuration supplies the seed stream;
// recurrence and delivery remain separate Primitives.
type Subtract[T core.Numeric] struct {
	core.PrimitiveError
	left, current core.Primitive
}

func NewSubtract[T core.Numeric](left core.Primitive) *Subtract[T] {
	return &Subtract[T]{current: store.NewRetained(nil), left: left}
}
func (operation *Subtract[T]) Next(in core.Primitive) core.Primitive {
	result := core.Yield(operation.left, in, func(held, value T) T { return held - value }, operation)
	transport.NewDiscard().Next(transport.NewApply(operation.current, transport.NewIO(result)))
	return result
}
func (operation *Subtract[T]) Read() any { return core.To[any](operation.current) }
