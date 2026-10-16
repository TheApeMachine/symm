package transport

import (
	"errors"
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Values lifts a fixed list of payloads into a run of unsafe.Pointer.
Next ignores the inbound run and yields a pointer to each held value;
repeated calls re-yield, producing fresh runs.
*/
type Values[T any] struct {
	err    error
	values []T
}

/*
NewValues instantiates a Values source Primitive from a fixed payload list.
The values are copied into storage the primitive owns: a variadic spread of
an existing slice aliases the caller's backing array, and downstream
primitives mutate their wire in place, so the source must own its storage.
*/
func NewValues[T any](values ...T) core.Primitive {
	owned := append([]T(nil), values...)

	return &Values[T]{values: owned}
}

func (op *Values[T]) Next(iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for index := range op.values {
			if !yield(unsafe.Pointer(&op.values[index])) {
				return
			}
		}
	}
}

func (op *Values[T]) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}

/*
One presents a single unsafe.Pointer as a run.
Next ignores the inbound run and yields the held pointer; repeated
calls re-yield, producing fresh runs.
*/
type One struct {
	err   error
	value unsafe.Pointer
}

/*
NewOne instantiates a One source Primitive from a single pointer.
*/
func NewOne(value unsafe.Pointer) core.Primitive {
	return &One{value: value}
}

func (op *One) Next(iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		yield(op.value)
	}
}

func (op *One) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}

/*
To evaluates a single input through an operation and returns the typed reading.
*/
func To[In any, Out any](op core.Primitive, in *In) Out {
	for ptr := range op.Next(func(yield func(unsafe.Pointer) bool) {
		yield(unsafe.Pointer(in))
	}) {
		if ptr != nil {
			return *(*Out)(ptr)
		}
	}
	var zero Out
	return zero
}

/*
Read extracts the first non-nil typed value from a sequence of unsafe.Pointers.
*/
func Read[T any](seq iter.Seq[unsafe.Pointer]) T {
	for ptr := range seq {
		if ptr != nil {
			return *(*T)(ptr)
		}
	}
	var zero T
	return zero
}

/*
NewValue instantiates a Primitive run yielding a pointer to a copy of v.
*/
func NewValue[T any](v T) core.Primitive {
	owned := v
	return &One{value: unsafe.Pointer(&owned)}
}

