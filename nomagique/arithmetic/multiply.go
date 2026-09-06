package arithmetic

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

// Multiply owns one field operation. Configuration supplies the seed stream;
// recurrence and delivery remain separate Primitives.
type Multiply[T core.Numeric] struct {
	core.PrimitiveError
	left, current core.Primitive
}

func NewMultiply[T core.Numeric](left core.Primitive) *Multiply[T] {
	return &Multiply[T]{current: store.NewRetained(nil), left: left}
}
func (operation *Multiply[T]) Next(in core.Primitive) core.Primitive {
	result := core.Yield(operation.left, in, func(held, value T) T { return held * value }, operation)
	transport.NewDiscard().Next(transport.NewApply(operation.current, transport.NewIO(result)))
	return result
}
func (operation *Multiply[T]) Read() any { return core.To[any](operation.current) }
