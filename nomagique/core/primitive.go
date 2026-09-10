package core

import (
	"errors"
	"iter"
)

/*
Primitive is the interface for a data cell that can read and write values of
different types, handle errors, and process incoming sequences of other
Primitives. It is parameterized by two types: T, the type of values it can
accept, and U, the type of values it holds and returns.
*/
type Primitive[T, U any] interface {
	Next(iter.Seq[Primitive[T, T]]) iter.Seq[Primitive[U, U]]
	Read() U
	Write(T)
	Error(...error) error
}

/*
Base is the standard Primitive[T, U] data cell. It holds a value of type U and
provides methods to read, write, and handle errors. The Base struct is designed
to be embedded in other structs that implement the Primitive interface.
*/
type Base[T, U any] struct {
	value U
	ok    bool
	err   error
	out   *Carrier[U] // Pre-allocated output carrier (zero-alloc per tick)
}

/*
Read returns the current value held by the Base. It is of type U, which may be
different from the type T that is written to it.
*/
func (base *Base[T, U]) Read() U { return base.value }

/*
Write updates the held value and returns a valid Primitive[U, U]
using the pre-allocated carrier struct—zero heap allocations in the loop.
*/
func (base *Base[T, U]) Write(value T) {
	base.value, base.ok = any(value).(U)
	if !base.ok {
		base.Error(ErrWrongType)
	}
}

/*
Error records the first error it sees and joins any subsequent errors to it.
*/
func (base *Base[T, U]) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil && base.err == nil {
			base.err = errors.Join(base.err, err)
		}
	}

	return base.err
}

/*
Carrier updates the held value and returns a valid Primitive[U, U]
using the pre-allocated carrier struct—zero heap allocations in the loop.
*/
func (base *Base[T, U]) Carrier(val U) Primitive[U, U] {
	if base.out == nil {
		base.out = &Carrier[U]{}
	}

	base.value = val
	base.out.value = val

	return base.out
}

/*
Carrier is the standard Primitive[T, T] data cell
*/
type Carrier[T any] struct {
	Base[T, T]
}

func (carrier *Carrier[T]) Next(in iter.Seq[Primitive[T, T]]) iter.Seq[Primitive[T, T]] {
	return in
}
