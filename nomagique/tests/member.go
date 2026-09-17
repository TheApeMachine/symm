package tests

import "github.com/theapemachine/symm/nomagique/core"

// Member gives real primitives an identity in store composition tests.
type Member[T any] struct {
	core.Primitive
	Address T
}

func (member *Member[T]) Identify(address T) core.Identifiable[T] {
	member.Address = address
	return member
}

func (member *Member[T]) Identity() T { return member.Address }
