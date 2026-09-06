package collection

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
	"math"
	"slices"
)

// Set replaces one indexed member without mutating its input collection. Index
// and replacement are configured Primitive sources. Numerical formulas deciding
// the replacement remain outside this structural operation.
type Set[T any] struct {
	core.PrimitiveError
	index, value, seed, current core.Primitive
}

func NewSet[T any](index, value core.Primitive) *Set[T] {
	return &Set[T]{index: index, value: value, seed: transport.NewIO(core.From([]T{})), current: store.NewRetained(nil)}
}
func (set *Set[T]) Next(in core.Primitive) core.Primitive {
	result := core.Yield(
		set.seed,
		in,
		func(_ []T, values []T) []T {
			index := -1
			core.Yield(
				transport.NewIO(core.From(0.0)),
				set.index,
				func(_ float64, value float64) float64 {
					if math.IsNaN(value) || math.IsInf(value, 0) || math.Trunc(value) != value || value < 0 || value >= float64(len(values)) {
						set.Error(core.ErrShape)
						return value
					}
					index = int(value)
					return value
				},
				set,
			)
			output := slices.Clone(values)
			observed := 0
			var replacement T
			core.Yield(
				transport.NewIO(core.NewProto(replacement)),
				set.value,
				func(_ T, value T) T { observed++; replacement = value; return value },
				set,
			)
			if index < 0 || observed != 1 {
				set.Error(core.ErrShape)
				return output
			}
			output[index] = replacement
			return output
		},
		set,
	)
	transport.NewDiscard().Next(transport.NewApply(set.current, transport.NewIO(result)))
	return result
}
func (set *Set[T]) Read() any { return core.To[any](set.current) }
