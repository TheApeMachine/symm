package collection

import (
	"cmp"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
	"slices"
)

// Order owns ordering a collection. It never reorders a caller's storage.
type Order[T cmp.Ordered] struct {
	core.PrimitiveError
	seed    core.Primitive
	current core.Primitive
}

func NewOrder[T cmp.Ordered]() *Order[T] {
	return &Order[T]{current: store.NewRetained(nil), seed: transport.NewIO(core.From([]T{}))}
}
func (order *Order[T]) Next(in core.Primitive) core.Primitive {
	result := core.Yield(order.seed, in, func(_, values []T) []T { out := slices.Clone(values); slices.Sort(out); return out }, order)
	transport.NewDiscard().Next(transport.NewApply(order.current, transport.NewIO(result)))
	return result
}
func (order *Order[T]) Read() any { return core.To[any](order.current) }
