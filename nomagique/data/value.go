package data

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

type Value[T any] struct {
	*core.PrimitiveError
	Values []unsafe.Pointer
}

func NewValue[T any](values ...T) *Value[T] {
	if ptrs, ok := any(values).([]unsafe.Pointer); ok {
		return &Value[T]{
			PrimitiveError: core.NewPrimitiveError(),
			Values:         ptrs,
		}
	}

	wrapped := make([]unsafe.Pointer, len(values))

	for i := range values {
		wrapped[i] = unsafe.Pointer(&values[i])
	}

	return &Value[T]{
		PrimitiveError: core.NewPrimitiveError(),
		Values:         wrapped,
	}
}

func (op *Value[T]) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for _, value := range op.Values {
			if !yield(value) {
				return
			}
		}
	}
}

func To[From, To any](op core.Primitive, payload *From) To {
	for out := range op.Next(NewValue(*payload).Next(nil)) {
		return *(*To)(out)
	}

	var zero To
	return zero
}

/*
Read takes the first value of a run out of the wire.
*/
func Read[T any](value iter.Seq[unsafe.Pointer]) T {
	var zero T

	if value == nil {
		return zero
	}

	for val := range value {
		if val == nil {
			continue
		}
		return *(*T)(val)
	}

	return zero
}

/*
Pull a value out of an iterator.
*/
func Pull[T any](value iter.Seq[T]) T {
	var zero T

	if value == nil {
		return zero
	}

	for val := range value {
		return val
	}

	return zero
}
