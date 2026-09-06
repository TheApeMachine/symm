package collection

import (
	"fmt"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
	"math"
)

// At selects an indexed member. Its index is a Primitive source, so an index
// can be computed by another composition without extending this operation.
type At[T any] struct {
	core.PrimitiveError
	index   core.Primitive
	seed    core.Primitive
	current core.Primitive
}

func NewAt[T any](index core.Primitive) *At[T] {
	var zero T
	return &At[T]{current: store.NewRetained(nil), index: index, seed: transport.NewIO(core.NewProto(zero))}
}
func (at *At[T]) Next(in core.Primitive) core.Primitive {
	result := core.Yield(
		at.seed,
		in,
		func(held T, values []T) T {
			index := -1
			core.Yield(
				transport.NewIO(core.From(0.0)),
				at.index,
				func(_, value float64) float64 {
					if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || math.Trunc(value) != value || value >= float64(int(^uint(0)>>1)) {
						at.Error(core.ErrShape)
						return value
					}
					index = int(value)
					return value
				},
				at,
			)
			if index < 0 || index >= len(values) {
				at.Error(fmt.Errorf("%w: index %d of %d", core.ErrShape, index, len(values)))
				return held
			}
			return values[index]
		},
		at,
	)
	transport.NewDiscard().Next(transport.NewApply(at.current, transport.NewIO(result)))
	return result
}
func (at *At[T]) Read() any { return core.To[any](at.current) }
