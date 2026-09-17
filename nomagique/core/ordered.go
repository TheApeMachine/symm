package core

// Ordered supplies a strict ordering for address indexes. Less must remain
// consistent while an identity is used as a stored key.
type Ordered[T any] interface {
	Less(T) bool
}
