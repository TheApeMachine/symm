package collection

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

// Append extends a collection supplied by the configured left connection.
// Persisting the output belongs to a composed storage Primitive.
type Append[T any] struct {
	core.PrimitiveError
	left, current core.Primitive
}

func NewAppend[T any](left core.Primitive) *Append[T] {
	return &Append[T]{current: store.NewRetained(nil), left: left}
}
func (a *Append[T]) Next(in core.Primitive) core.Primitive {
	result := core.Yield(a.left, in, func(held []T, value T) []T { return append(held, value) }, a)
	transport.NewDiscard().Next(transport.NewApply(a.current, transport.NewIO(result)))
	return result
}
func (a *Append[T]) Read() any { return core.To[any](a.current) }
