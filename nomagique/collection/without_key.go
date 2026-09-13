package collection

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
	err error
	out []Keyed[K, V]
}

func NewWithoutKey[K comparable, V any]() core.Primitive {
	return &Without[K, V]{}
}

func (op *Without[K, V]) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			input := *(*WithoutKeyInput[K, V])(arriving)
			op.out = make([]Keyed[K, V], 0, len(input.Values))

			for _, member := range input.Values {
				if member.Key != input.Key {
					op.out = append(op.out, member)
				}
			}

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

func (op *Without[K, V]) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = err
			break
		}
	}

	return op.err
}
