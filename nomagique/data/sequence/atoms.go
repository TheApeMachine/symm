package sequence

import (
	"cmp"
	"slices"

	"github.com/theapemachine/symm/nomagique/types"
)

type Window[T any] types.Value[T, []T]
/*
NewWindow creates a stateful rolling window closure.
It accumulates incoming items up to size, emitting the current window slice.
No structs, pure Value closure.
*/
func NewWindow[T any](size int) Window[T] {
	buf := make([]T, 0, size)
	return func(in T) []T {
		if len(buf) >= size {
			buf = buf[1:]
		}
		buf = append(buf, in)
		out := make([]T, len(buf))
		copy(out, buf)
		return out
	}
}

type Tail[T any] types.Value[[]T, []T]
/*
NewTail creates a closure returning the last N items of a slice.
*/
func NewTail[T any](size int) Tail[T] {
	return func(in []T) []T {
		if len(in) <= size {
			out := make([]T, len(in))
			copy(out, in)
			return out
		}
		out := make([]T, size)
		copy(out, in[len(in)-size:])
		return out
	}
}

type At[T any] types.Value[[]T, T]
/*
NewAt returns the item at index in the slice.
*/
func NewAt[T any](index int) At[T] {
	return func(in []T) T {
		var zero T
		if index < 0 || index >= len(in) {
			return zero
		}
		return in[index]
	}
}

type Values[T any] types.Value[struct{}, []T]
/*
NewValues returns a constant slice supplier.
*/
func NewValues[T any](values ...T) Values[T] {
	return func(struct{}) []T {
		out := make([]T, len(values))
		copy(out, values)
		return out
	}
}

type Order[T cmp.Ordered] types.Value[[]T, []T]
/*
NewOrder returns a closure that sorts a slice of ordered items in ascending order.
*/
func NewOrder[T cmp.Ordered]() Order[T] {
	return func(in []T) []T {
		out := make([]T, len(in))
		copy(out, in)
		slices.Sort(out)
		return out
	}
}

type Append[T any] types.Value[[2]any, []T]
/*
NewAppend returns a closure that appends an item to a slice.
*/
func NewAppend[T any]() Append[T] {
	return func(in [2]any) []T {
		s, _ := in[0].([]T)
		val, _ := in[1].(T)
		return append(s, val)
	}
}
