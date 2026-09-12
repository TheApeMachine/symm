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
