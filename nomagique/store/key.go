package store

import (
	"errors"
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Key associates each arrival with its configured key. Repeated writes to the
same key in a run retain the last value, exactly as KV does.
*/
type Key[K comparable, V any] struct {
	err error
	key K
	out map[K]V
}

func NewKey[K comparable, V any](key K) core.Primitive {
	return &Key[K, V]{key: key}
}

func (op *Key[K, V]) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			val := *(*V)(arriving)
			op.out = map[K]V{op.key: val}

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

func (op *Key[K, V]) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}
