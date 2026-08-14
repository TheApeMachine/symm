package types

/*
Pair is one named position: a comparable key and a comparable value.
It is one entry, not a collection.
*/
type Pair[K comparable, V comparable] struct {
	Key   K `json:"key"`
	Value V `json:"value"`
}

func NewPair[K comparable, V comparable](key K, value V) Pair[K, V] {
	return Pair[K, V]{
		Key:   key,
		Value: value,
	}
}
