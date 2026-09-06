package transport

import "github.com/theapemachine/symm/nomagique/core"

// Collect retains the values of one run as one collection. It neither converts
// elements nor assigns domain meanings to them.
type Collect[T any] struct {
	core.PrimitiveError
	seed    core.Primitive
	current core.Primitive
}

func NewCollect[T any]() *Collect[T] {
	return &Collect[T]{seed: NewIO(core.From([]T{}))}
}
func (collect *Collect[T]) Next(in core.Primitive) core.Primitive {
	collect.current = core.Yield(collect.seed, in, func(held []T, value T) []T {
		return append(held, value)
	}, collect)
	return collect.current
}
func (collect *Collect[T]) Read() any { return core.To[any](collect.current) }
