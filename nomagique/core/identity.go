package core

import (
	"iter"
	"unsafe"
)

/*
Identifiable provides a way to reference or address external objects.
This can be used to earmark output to a certain external resource,
for example.
*/
type Identifiable[T any] interface {
	Primitive
	Identify(T) Identifiable[T]
	Identity() T
}

type ProtoIdentity[T any] struct {
	*PrimitiveError
	address T
	wrapped []Primitive
}

func NewProtoIdentity[T any](wrapped ...Primitive) *ProtoIdentity[T] {
	return &ProtoIdentity[T]{
		PrimitiveError: NewPrimitiveError(),
		wrapped:        wrapped,
	}
}

func (pi *ProtoIdentity[T]) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		stream := in
		for _, wrapper := range pi.wrapped {
			stream = wrapper.Next(stream)
		}

		for out := range stream {
			if !yield(out) {
				return
			}
		}
	}
}

func (pi *ProtoIdentity[T]) Identify(identifier T) Identifiable[T] {
	pi.address = identifier
	return pi
}

func (pi *ProtoIdentity[T]) Identity() T {
	return pi.address
}
