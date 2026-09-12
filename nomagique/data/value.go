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

	for val := range value {
		return *(*T)(val)
	}

	return zero
}
