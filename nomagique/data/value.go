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
	wrapped := make([]unsafe.Pointer, len(values))

	for i := range values {
		wrapped[i] = unsafe.Pointer(&values[i])
	}

	return &Value[T]{
		PrimitiveError: core.NewPrimitiveError(),
		Values:         wrapped,
	}
}

func NewPointers(values ...unsafe.Pointer) *Value[unsafe.Pointer] {
	return &Value[unsafe.Pointer]{
		PrimitiveError: core.NewPrimitiveError(),
		Values:         values,
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

func To[T any](value unsafe.Pointer) T {
	return *(*T)(value)
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

/*
ReadSeq walks all values of a run out of the wire as typed values.
*/
func ReadSeq[T any](value iter.Seq[unsafe.Pointer]) iter.Seq[T] {
	if value == nil {
		return func(yield func(T) bool) {}
	}

	return func(yield func(T) bool) {
		for val := range value {
			if val == nil {
				continue
			}
			if !yield(*(*T)(val)) {
				return
			}
		}
	}
}
