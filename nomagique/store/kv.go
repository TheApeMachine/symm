package store

import (
	"github.com/theapemachine/symm/nomagique/core"
	"maps"
)

// KV associates incoming keys and values with a configured map source. It never
// mutates that source. Retaining successive maps is explicit feedback through
// Retained; a fresh seed instead builds an independent record on every run.
type KV[K comparable] struct {
	core.PrimitiveError
	left, current core.Primitive
}

func NewKV[K comparable](left core.Primitive) *KV[K] {
	return &KV[K]{left: left}
}
func (kv *KV[K]) Next(in core.Primitive) core.Primitive {
	result := core.Yield(
		kv.left,
		in,
		func(held, arriving map[K]core.Primitive) map[K]core.Primitive {
			merged := make(map[K]core.Primitive, len(held)+len(arriving))
			maps.Copy(merged, held)
			maps.Copy(merged, arriving)
			return merged
		},
		kv,
	)
	if result != nil {
		kv.current = result
	}
	return result
}
func (kv *KV[K]) Read() any { return core.To[any](kv.current) }
