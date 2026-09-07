package arithmetic

import (
	"github.com/theapemachine/symm/nomagique/core"
)

// Add owns one field operation. Configuration supplies the seed stream;
// recurrence and delivery remain separate Primitives.
type Add[T core.Numeric] struct {
	core.PrimitiveError
	left, current core.Primitive
}

func NewAdd[T core.Numeric](left core.Primitive) *Add[T] {
	return &Add[T]{left: left}
}
func (operation *Add[T]) Next(in core.Primitive) core.Primitive {
	result := core.Yield(operation.left, in, func(held, value T) T { return held + value }, operation)
	if result != nil {
		operation.current = result
	}
	return result
}
func (operation *Add[T]) Read() any { return core.To[any](operation.current) }
