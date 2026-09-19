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
func NewWindow[T any](size types.Integer) Window[T] {
	var buf []T
	return func(in T) []T {
		s := 10
		if size != nil {
			if evaluated := size(in); evaluated > 0 {
				s = evaluated
			}
		}
		if len(buf) >= s {
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
func NewTail[T any](size types.Integer) Tail[T] {
	return func(in []T) []T {
		s := 10
		if size != nil {
			if evaluated := size(in); evaluated > 0 {
				s = evaluated
			}
		}
		if len(in) <= s {
			out := make([]T, len(in))
			copy(out, in)
			return out
		}
		out := make([]T, s)
		copy(out, in[len(in)-s:])
		return out
	}
}

type At[T any] types.Value[[]T, T]
/*
NewAt returns the item at index in the slice.
*/
func NewAt[T any](index types.Integer) At[T] {
	return func(in []T) T {
		var zero T
		idx := 0
		if index != nil {
			idx = index(in)
		}
		if idx < 0 || idx >= len(in) {
			return zero
		}
		return in[idx]
	}
}

type Values[T any] types.Value[any, []T]
/*
NewValues returns a constant slice supplier.
*/
func NewValues[T any](values ...types.Value[any, T]) Values[T] {
	return func(in any) []T {
		out := make([]T, len(values))
		for i, v := range values {
			if v != nil {
				out[i] = v(in)
			}
		}
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

type Append[T any] types.Value[any, []T]
/*
NewAppend returns a closure that appends an item to a slice.
*/
func NewAppend[T any](slice types.Value[any, []T], item types.Value[any, T]) Append[T] {
	return func(in any) []T {
		var s []T
		if slice != nil {
			s = slice(in)
		} else if arr, ok := in.([]T); ok {
			s = arr
		} else if pair, ok := in.([2]any); ok {
			s, _ = pair[0].([]T)
			val, _ := pair[1].(T)
			return append(s, val)
		}
		var val T
		if item != nil {
			val = item(in)
		}
		return append(s, val)
	}
}
