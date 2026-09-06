package arithmetic

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

// Divide owns one field operation. Configuration supplies the seed stream;
// recurrence and delivery remain separate Primitives.
type Divide[T core.Floating] struct {
	core.PrimitiveError
	left, current core.Primitive
}

func NewDivide[T core.Floating](left core.Primitive) *Divide[T] {
	return &Divide[T]{current: store.NewRetained(nil), left: left}
}
func (operation *Divide[T]) Next(in core.Primitive) core.Primitive {
	result := core.Yield(operation.left, in, func(held, value T) T { return held / value }, operation)
	transport.NewDiscard().Next(transport.NewApply(operation.current, transport.NewIO(result)))
	return result
}
func (operation *Divide[T]) Read() any { return core.To[any](operation.current) }
