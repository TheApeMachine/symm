package sequence

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Keyed is one keyed member of a collection. It carries facts between
primitives and computes nothing.
*/
type Keyed[K comparable, V any] struct {
	Key   K
	Value V
}

/*
WithoutKeyInput is one keyed collection with the key to exclude.
*/
type WithoutKeyInput[K comparable, V any] struct {
	Key    K
	Values []Keyed[K, V]
}

/*
Without owns the exclusion.
*/
type Without[K comparable, V any] struct {
	*core.PrimitiveError

	out []Keyed[K, V]
}

func NewWithoutKey[K comparable, V any]() *Without[K, V] {
	return &Without[K, V]{PrimitiveError: core.NewPrimitiveError()}
}

func (without *Without[K, V]) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			input := *(*WithoutKeyInput[K, V])(arriving)
			without.out = make([]Keyed[K, V], 0, len(input.Values))

			for _, member := range input.Values {
				if member.Key != input.Key {
					without.out = append(without.out, member)
				}
			}

			if !yield(unsafe.Pointer(&without.out)) {
				return
			}
		}
	}
}
