package arithmetic

import (
	"github.com/theapemachine/symm/nomagique/core"
)

// Multiply owns one field operation. Configuration supplies the seed stream;
// recurrence and delivery remain separate Primitives.
type Multiply[T core.Numeric] struct {
	core.PrimitiveError
	left, current core.Primitive
}

func NewMultiply[T core.Numeric](left core.Primitive) *Multiply[T] {
	return &Multiply[T]{left: left}
}
func (operation *Multiply[T]) Next(in core.Primitive) core.Primitive {
	result := core.Yield(operation.left, in, func(held, value T) T { return held * value }, operation)
	if result != nil {
		operation.current = result
	}
	return result
}
func (operation *Multiply[T]) Read() any { return core.To[any](operation.current) }
