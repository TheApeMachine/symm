package logic

import (
	"cmp"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

// LessEqual owns the non-strict ordering relation.
type LessEqual[T cmp.Ordered] struct {
	core.PrimitiveError
	seed, current core.Primitive
}

func NewLessEqual[T cmp.Ordered]() *LessEqual[T] {
	return &LessEqual[T]{current: store.NewRetained(nil), seed: transport.NewIO(core.From(false))}
}
func (operation *LessEqual[T]) Next(in core.Primitive) core.Primitive {
	result := core.Yield(
		operation.seed,
		in,
		func(_ bool, v []T) bool {
			if len(v) != 2 {
				operation.Error(core.ErrShape)
				return false
			}
			return v[0] <= v[1]
		},
		operation,
	)
	transport.NewDiscard().Next(transport.NewApply(operation.current, transport.NewIO(result)))
	return result
}
func (operation *LessEqual[T]) Read() any { return core.To[any](operation.current) }
