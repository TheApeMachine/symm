package collection

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
	"slices"
)

// Tail selects the last configured number of collection members. Capacity is
// supplied as an integer Primitive, never inferred from the payload's use case.
type Tail[T any] struct {
	core.PrimitiveError
	capacity, seed, current core.Primitive
}

func NewTail[T any](capacity core.Primitive) *Tail[T] {
	return &Tail[T]{current: store.NewRetained(nil), capacity: capacity, seed: transport.NewIO(core.From([]T{}))}
}
func (t *Tail[T]) Next(in core.Primitive) core.Primitive {
	result := core.Yield(
		t.seed,
		in,
		func(_ []T, value []T) []T {
			count := 0
			core.Yield(transport.NewIO(core.From(0)), t.capacity, func(_ int, n int) int { count = n; return n }, t)
			if count < 0 {
				t.Error(core.ErrShape)
				return nil
			}
			return slices.Clone(value[max(0, len(value)-count):])
		},
		t,
	)
	transport.NewDiscard().Next(transport.NewApply(t.current, transport.NewIO(result)))
	return result
}
func (t *Tail[T]) Read() any { return core.To[any](t.current) }
