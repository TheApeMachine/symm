package data

import (
	"iter"
	"unsafe"
)

/*
NewValue lifts values into a run; the run yields pointers to the values, so a
store receives an addressable payload and answers into the same memory.
*/
func NewValue[T any](value ...T) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for index := range value {
			if !yield(unsafe.Pointer(&value[index])) {
				return
			}
		}
	}
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

