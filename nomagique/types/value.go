package types

import "github.com/theapemachine/errnie"

/*
Value is the abstraction of a Go primitive comparable value.
It turns the primitive into a nomagique compatible type.
*/
type Value[T comparable] struct {
	Zero    T     `json:"zero"`
	Initial T     `json:"initial"`
	Next    T     `json:"next"`
	Ready   bool  `json:"ready"`
	hasNext bool
	Err     error `json:"err"`
}

/*
NewValue returns a ready payload.
*/
func NewValue[T comparable](initial T) Value[T] {
	return Value[T]{
		Initial: initial,
		Ready:   true,
	}
}

/*
Read returns current, unless next has been staged. In that case
it first swaps next into current, then returns current.
*/
func (value Value[T]) Read() T {
	if !value.Ready {
		value.Err = errnie.Error(errnie.Err(
			errnie.NotFound,
			"value: input has not been written",
			nil,
		))

		return value.Zero
	}

	if value.Err != nil {
		return value.Zero
	}

	if !value.hasNext {
		return value.Initial
	}

	return value.Next
}

/*
Write stages a payload and returns the updated Value.
*/
func (value Value[T]) Write(next T) Value[T] {
	value.Next = next
	value.hasNext = true
	value.Ready = true

	return value
}

/*
Reset clears the staged payload.
*/
func (value *Value[T]) Reset() error {
	var zero T
	value.Next = zero
	value.hasNext = false
	value.Ready = true
	value.Err = nil

	return nil
}

/*
Close releases staged state.
*/
func (value *Value[T]) Close() error {
	return value.Reset()
}

/*
Error reports a write or read failure.
*/
func (value *Value[T]) Error() string {
	if value.Err == nil {
		return ""
	}

	return value.Err.Error()
}
