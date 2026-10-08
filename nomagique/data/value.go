package data

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

type Value[T any] struct {
	*core.PrimitiveError
	Values []T
}

func NewValue[T any](values ...T) *Value[T] {
	return &Value[T]{
		PrimitiveError: core.NewPrimitiveError(),
		Values:         values,
	}
}

func (op *Value[T]) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		var (
			items    []unsafe.Pointer
			buffered iter.Seq[unsafe.Pointer]
		)

		if in != nil {
			for arriving := range in {
				items = append(items, arriving)
			}

			buffered = func(subYield func(unsafe.Pointer) bool) {
				for _, item := range items {
					if !subYield(item) {
						return
					}
				}
			}
		}

		for _, value := range op.Values {
			if buffered != nil {
				if p, ok := any(value).(core.Primitive); ok && p != nil {
					for out := range p.Next(buffered) {
						if !yield(out) {
							return
						}
					}

					continue
				}
			}

			if ptr, ok := any(value).(unsafe.Pointer); ok {
				if !yield(ptr) {
					return
				}
				continue
			}

			val := new(T)
			*val = value

			if !yield(unsafe.Pointer(val)) {
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
