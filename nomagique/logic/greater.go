package logic

import (
	"cmp"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

// Greater compares two ordered payloads; pairing and routing are external.
type Greater[T cmp.Ordered] struct {
	core.PrimitiveError
	seed, current core.Primitive
}

func NewGreater[T cmp.Ordered]() *Greater[T] {
	return &Greater[T]{current: store.NewRetained(nil), seed: transport.NewIO(core.From(false))}
}
func (operation *Greater[T]) Next(in core.Primitive) core.Primitive {
	result := core.Yield(
		operation.seed,
		in,
		func(_ bool, values []T) bool {
			if len(values) != 2 {
				operation.Error(core.ErrShape)
				return false
			}
			return values[0] > values[1]
		},
		operation,
	)
	transport.NewDiscard().Next(transport.NewApply(operation.current, transport.NewIO(result)))
	return result
}
func (operation *Greater[T]) Read() any { return core.To[any](operation.current) }
