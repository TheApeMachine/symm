package logic

import (
	"cmp"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

// Equal compares two ordered payloads; pairing and routing are external.
type Equal[T cmp.Ordered] struct {
	core.PrimitiveError
	seed, current core.Primitive
}

func NewEqual[T cmp.Ordered]() *Equal[T] {
	return &Equal[T]{current: store.NewRetained(nil), seed: transport.NewIO(core.From(false))}
}
func (operation *Equal[T]) Next(in core.Primitive) core.Primitive {
	result := core.Yield(
		operation.seed,
		in,
		func(_ bool, values []T) bool {
			if len(values) != 2 {
				operation.Error(core.ErrShape)
				return false
			}
			return values[0] == values[1]
		},
		operation,
	)
	transport.NewDiscard().Next(transport.NewApply(operation.current, transport.NewIO(result)))
	return result
}
func (operation *Equal[T]) Read() any { return core.To[any](operation.current) }
