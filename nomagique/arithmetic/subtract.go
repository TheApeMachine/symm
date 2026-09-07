package arithmetic

import (
	"github.com/theapemachine/symm/nomagique/core"
)

// Subtract owns one field operation. Configuration supplies the seed stream;
// recurrence and delivery remain separate Primitives.
type Subtract[T core.Numeric] struct {
	core.PrimitiveError
	left, current core.Primitive
}

func NewSubtract[T core.Numeric](left core.Primitive) *Subtract[T] {
	return &Subtract[T]{left: left}
}
func (operation *Subtract[T]) Next(in core.Primitive) core.Primitive {
	result := core.Yield(operation.left, in, func(held, value T) T { return held - value }, operation)
	if result != nil {
		operation.current = result
	}
	return result
}
func (operation *Subtract[T]) Read() any { return core.To[any](operation.current) }
