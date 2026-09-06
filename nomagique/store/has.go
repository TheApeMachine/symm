package store

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/transport"
)

// Has owns key membership. Missing data can be routed before Get is applied;
// lookup itself continues to reject absent keys rather than inventing a zero.
type Has[K comparable] struct {
	core.PrimitiveError
	key           K
	seed, current core.Primitive
}

func NewHas[K comparable](key K) *Has[K] {
	return &Has[K]{key: key, seed: transport.NewIO(core.From(false)), current: NewRetained(nil)}
}
func (has *Has[K]) Next(in core.Primitive) core.Primitive {
	result := core.Yield(has.seed, in, func(_ bool, values map[K]core.Primitive) bool { _, present := values[has.key]; return present }, has)
	transport.NewDiscard().Next(transport.NewApply(has.current, transport.NewIO(result)))
	return result
}
func (has *Has[K]) Read() any { return core.To[any](has.current) }
