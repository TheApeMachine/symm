package store

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
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
	*core.PrimitiveError

	current map[K]V
	prior   map[K]V
	has     map[K]bool
	out     LatestReading[K, V]
	snap    map[K]V
}

func NewLatest[K comparable, V any]() *Latest[K, V] {
	return &Latest[K, V]{PrimitiveError: core.NewPrimitiveError(), current: make(map[K]V),
		prior: make(map[K]V),
		has:   make(map[K]bool),
	}
}

func (latest *Latest[K, V]) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			command := *(*LatestCommand[K, V])(arriving)

			if command.Read {
				latest.snap = make(map[K]V, len(latest.current))

				for key, value := range latest.current {
					latest.snap[key] = value
				}

				if !yield(unsafe.Pointer(&latest.snap)) {
					return
				}

				continue
			}

			reading := LatestReading[K, V]{Key: command.Key, Current: command.Value}
			reading.Prior, reading.HasPrior = latest.current[command.Key], latest.has[command.Key]

			latest.prior[command.Key] = latest.current[command.Key]
			latest.current[command.Key] = command.Value
			latest.has[command.Key] = true

			latest.out = reading

			if !yield(unsafe.Pointer(&latest.out)) {
				return
			}
		}
	}
}
