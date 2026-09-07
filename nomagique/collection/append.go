package collection

import (
	"github.com/theapemachine/symm/nomagique/core"
)

// Append extends a collection supplied by the configured left connection.
// Persisting the output belongs to a composed storage Primitive.
type Append[T any] struct {
	core.PrimitiveError
	left, current core.Primitive
}

func NewAppend[T any](left core.Primitive) *Append[T] {
	return &Append[T]{left: left}
}
func (a *Append[T]) Next(in core.Primitive) core.Primitive {
	result := core.Yield(a.left, in, func(held []T, value T) []T { return append(held, value) }, a)
	if result != nil {
		a.current = result
	}
	return result
}
func (a *Append[T]) Read() any { return core.To[any](a.current) }
