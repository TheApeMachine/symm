package store

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/transport"
)

// Key associates each delivered value with its configured key. Repeated writes
// to the same key in a run retain the last value, exactly as KV does.
type Key[K comparable] struct {
	core.PrimitiveError
	key     K
	seed    core.Primitive
	current core.Primitive
}

func NewKey[K comparable](key K) *Key[K] {
	return &Key[K]{current: NewRetained(nil), key: key, seed: transport.NewIO(core.From(map[K]core.Primitive{}))}
}
func (key *Key[K]) Next(in core.Primitive) core.Primitive {
	result := core.Yield(
		key.seed,
		in,
		func(_ map[K]core.Primitive, value core.Primitive) map[K]core.Primitive {
			return map[K]core.Primitive{key.key: value}
		},
		key,
	)
	transport.NewDiscard().Next(transport.NewApply(key.current, transport.NewIO(result)))
	return result
}
func (key *Key[K]) Read() any { return core.To[any](key.current) }
