package store

import (
	"iter"
	"unsafe"

)

/*
LatestCommand is one intent against the keyed latest store: a write replaces
a key's value; a read asks for the retained snapshot.
*/
type LatestCommand[K comparable, V any] struct {
	Read  bool
	Key   K
	Value V
}

/*
LatestReading answers a write: the key, the value now current, and the value
it replaced so downstream composition can form a causal change.
*/
type LatestReading[K comparable, V any] struct {
	Key      K
	Current  V
	Prior    V
	HasPrior bool
}

/*
Latest owns keyed retention: one latest value per key, and nothing else. A
write yields the reading with the value it replaced; a read yields the
retained snapshot. It knows nothing about why the caller retains values.
*/
type Latest[K comparable, V any] struct {
	err     error
	current map[K]V
	prior   map[K]V
	has     map[K]bool
	out     LatestReading[K, V]
	snap    map[K]V
}

func NewLatest[K comparable, V any]() *Latest[K, V] {
	return &Latest[K, V]{
		current: make(map[K]V),
		prior:   make(map[K]V),
		has:     make(map[K]bool),
	}
}

func (op *Latest[K, V]) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			command := *(*LatestCommand[K, V])(arriving)

			if command.Read {
				op.snap = make(map[K]V, len(op.current))

				for key, value := range op.current {
					op.snap[key] = value
				}

				if !yield(unsafe.Pointer(&op.snap)) {
					return
				}

				continue
			}

			reading := LatestReading[K, V]{Key: command.Key, Current: command.Value}
			reading.Prior, reading.HasPrior = op.current[command.Key], op.has[command.Key]

			op.prior[command.Key] = op.current[command.Key]
			op.current[command.Key] = command.Value
			op.has[command.Key] = true

			op.out = reading

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

func (op *Latest[K, V]) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = err
			break
		}
	}

	return op.err
}
