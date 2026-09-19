package store

import "github.com/theapemachine/symm/nomagique/types"

/*
NewConstant replaces each arrival with a configured value.
No structs, pure Value closure.
*/
type Constant[T, Any any] types.Value[Any, T]
func NewConstant[T, Any any](current types.Value[Any, T]) Constant[T, Any] {
	return func(in Any) T {
		if current != nil {
			return current(in)
		}
		var zero T
		return zero
	}
}
