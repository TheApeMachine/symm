package types

import (
	"fmt"
	"iter"
	"maps"
)

/*
Map is a named collection of comparable pairs. The Go map lives behind a
pointer so the Map struct itself is comparable and can be IO's T.
Content equality is Equals.
*/
type Map[K comparable, V comparable] struct {
	store *map[K]V
}

func NewMap[K comparable, V comparable]() Map[K, V] {
	entries := make(map[K]V)

	return Map[K, V]{
		store: &entries,
	}
}

func (mapping Map[K, V]) Get(key K) (V, bool) {
	var zero V

	if mapping.store == nil {
		return zero, false
	}

	value, found := (*mapping.store)[key]

	return value, found
}

func (mapping Map[K, V]) Put(key K, value V) {
	if mapping.store == nil {
		panic("types.Map.Put: zero map; use types.NewMap")
	}

	(*mapping.store)[key] = value
}

/*
Delete removes one pair from the collection.
*/
func (mapping Map[K, V]) Delete(key K) {
	if mapping.store == nil {
		return
	}

	delete(*mapping.store, key)
}

/*
Len returns the number of retained pairs.
*/
func (mapping Map[K, V]) Len() int {
	if mapping.store == nil {
		return 0
	}

	return len(*mapping.store)
}

/*
All exposes the collection as an iterator without exposing its backing map.
*/
func (mapping Map[K, V]) All() iter.Seq2[K, V] {
	return func(yield func(K, V) bool) {
		if mapping.store == nil {
			return
		}

		for key, value := range *mapping.store {
			if !yield(key, value) {
				return
			}
		}
	}
}

/*
Clone returns an independently mutable collection with the same pairs.
*/
func (mapping Map[K, V]) Clone() Map[K, V] {
	cloned := NewMap[K, V]()

	for key, value := range mapping.All() {
		cloned.Put(key, value)
	}

	return cloned
}

func (mapping Map[K, V]) Equals(other Map[K, V]) bool {
	var left map[K]V
	var right map[K]V

	if mapping.store != nil {
		left = *mapping.store
	}

	if other.store != nil {
		right = *other.store
	}

	return maps.Equal(left, right)
}

/*
Validate if the mapping contains all the provided keys.
*/
func (mapping Map[K, V]) Validate(keys ...K) (bool, []string) {
	missing := make([]string, 0)

	for _, key := range keys {
		if _, found := mapping.Get(key); !found {
			missing = append(missing, fmt.Sprintf("%v", key))
		}
	}

	return len(missing) == 0, missing
}
