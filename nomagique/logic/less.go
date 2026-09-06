package logic

import (
	"cmp"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

// Less compares two ordered payloads; pairing and routing are external.
type Less[T cmp.Ordered] struct {
	core.PrimitiveError
	seed, current core.Primitive
}

func NewLess[T cmp.Ordered]() *Less[T] {
	return &Less[T]{current: store.NewRetained(nil), seed: transport.NewIO(core.From(false))}
}
func (operation *Less[T]) Next(in core.Primitive) core.Primitive {
	result := core.Yield(
		operation.seed,
		in,
		func(_ bool, values []T) bool {
			if len(values) != 2 {
				operation.Error(core.ErrShape)
				return false
			}
			return values[0] < values[1]
		},
		operation,
	)
	transport.NewDiscard().Next(transport.NewApply(operation.current, transport.NewIO(result)))
	return result
}
func (operation *Less[T]) Read() any { return core.To[any](operation.current) }
