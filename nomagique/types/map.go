package types

import "maps"

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
	(*mapping.store)[key] = value
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
