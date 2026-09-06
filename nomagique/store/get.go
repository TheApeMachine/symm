package store

import (
	"fmt"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/transport"
)

// Get owns lookup, not a formula or metadata interpretation. Missing keys are
// explicit failures, never a fabricated numerical zero.
type Get[K comparable] struct {
	core.PrimitiveError
	key     K
	seed    core.Primitive
	current core.Primitive
}

func NewGet[K comparable](key K) *Get[K] {
	return &Get[K]{current: NewRetained(nil), key: key, seed: transport.NewIO(core.NewProto(nil))}
}
func (get *Get[K]) Next(in core.Primitive) core.Primitive {
	result := core.Yield(
		get.seed,
		in,
		func(_ core.Primitive, values map[K]core.Primitive) core.Primitive {
			value, found := values[get.key]
			if !found {
				get.Error(fmt.Errorf("%w: key %v", core.ErrNotHeld, get.key))
				return core.NewProto(nil)
			}
			return value
		},
		get,
	)
	transport.NewDiscard().Next(transport.NewApply(get.current, transport.NewIO(result)))
	return result
}
func (get *Get[K]) Read() any { return core.To[any](get.current) }
